// Package mcp 实现一个极简 MCP（Model Context Protocol）客户端，
// 支持 Streamable HTTP 与 SSE 两种远程传输（本地 stdio 不在本版本支持范围）。
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Tool 是 MCP 服务器提供的工具描述。
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

// toolCallResult 是 tools/call 的返回结构。
type toolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// rpcRequest / rpcResponse 是 JSON-RPC 2.0 报文。
type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int        `json:"id,omitempty"` // 通知（notifications/）不带 id
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// protocolVersions 按优先级尝试的协议版本（服务器拒绝时逐级回退）。
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

const rpcTimeout = 60 * time.Second

// Client 是一个已初始化的 MCP 连接。
type Client struct {
	baseURL string
	headers map[string]string
	http    *http.Client
	sseHTTP *http.Client // SSE 模式专用（无超时，长连接）

	mu      sync.Mutex
	nextID  int
	session string // Mcp-Session-Id（服务器可能要求回传）

	// SSE 类型专用
	sseMode     bool
	sseMu       sync.Mutex
	ssePending  map[int]chan rpcResponse
	sseEarly    map[int]rpcResponse // 先于等待者到达的响应
	sseEndpoint string
	sseReady    chan struct{} // endpoint 就绪
	sseCancel   context.CancelFunc
}

// NewClient 创建一个 MCP 客户端（尚未连接）。
func NewClient(rawURL string, headers map[string]string, sseMode bool) *Client {
	h := make(map[string]string, len(headers))
	for k, v := range headers {
		h[k] = v
	}
	return &Client{
		baseURL:    strings.TrimRight(rawURL, "/"),
		headers:    h,
		http:       &http.Client{Timeout: rpcTimeout},
		sseHTTP:    &http.Client{},
		sseMode:    sseMode,
		ssePending: map[int]chan rpcResponse{},
		sseEarly:   map[int]rpcResponse{},
	}
}

// DetectType 根据 mcp.json 条目推断传输类型：返回 "sse"、"streamable" 或 "stdio"。
func DetectType(typ, rawURL, command string) string {
	t := strings.ToLower(strings.TrimSpace(typ))
	switch t {
	case "sse":
		return "sse"
	case "http", "streamablehttp", "streamable-http", "streamable_http":
		return "streamable"
	}
	if command != "" {
		return "stdio"
	}
	u := strings.ToLower(rawURL)
	if strings.HasSuffix(u, "/sse") || strings.HasSuffix(u, "sse") {
		return "sse"
	}
	return "streamable"
}

func (c *Client) nextReqID() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	return c.nextID
}

func (c *Client) sessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.session
}

func (c *Client) setSession(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == "" {
		c.session = id
	}
}

// Initialize 完成 MCP 握手；协议版本逐级回退以兼容不同服务器。
func (c *Client) Initialize(ctx context.Context) error {
	var lastErr error
	for _, v := range protocolVersions {
		err := c.handshake(ctx, v)
		if err == nil {
			return nil
		}
		lastErr = err
		// 仅当服务器明确表示协议版本不受支持时才尝试更老的版本
		if !strings.Contains(strings.ToLower(err.Error()), "protocol version") &&
			!strings.Contains(strings.ToLower(err.Error()), "unsupported mcp") {
			return err
		}
	}
	if lastErr == nil {
		lastErr = errors.New("握手失败")
	}
	return lastErr
}

func (c *Client) handshake(ctx context.Context, version string) error {
	if c.sseMode {
		if err := c.startSSE(ctx); err != nil {
			return err
		}
	}
	res, err := c.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": version,
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "mcppreflight", "version": "1.0.0"},
	})
	if err != nil {
		return err
	}
	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	_ = json.Unmarshal(res, &initResult)
	// 初始化完成通知（服务器通常返回 202，忽略错误不影响后续）
	_, _ = c.call(ctx, "notifications/initialized", nil)
	return nil
}

// call 发送一次 JSON-RPC 请求并等待结果；notifications/ 方法只发送不等待。
func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextReqID()
	isNotification := strings.HasPrefix(method, "notifications/")
	var idPtr *int
	if !isNotification {
		idPtr = &id
	}
	reqBody := rpcRequest{JSONRPC: "2.0", ID: idPtr, Method: method, Params: params}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	if isNotification {
		// 通知：发送即忘，不等待响应
		if c.sseMode {
			c.postNotification(ctx, b)
		} else {
			c.postNotificationHTTP(ctx, b)
		}
		return nil, nil
	}
	if c.sseMode {
		return c.callSSE(ctx, id, b)
	}
	return c.callHTTP(ctx, b, id)
}

// postNotificationHTTP 以 Streamable HTTP 发送通知（忽略结果）。
func (c *Client) postNotificationHTTP(ctx context.Context, body []byte) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if sid := c.sessionID(); sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	resp.Body.Close()
}

// callHTTP 通过 Streamable HTTP 发送请求。
func (c *Client) callHTTP(ctx context.Context, body []byte, wantID int) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	if sid := c.sessionID(); sid != "" {
		req.Header.Set("Mcp-Session-Id", sid)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 MCP 服务器失败: %w", err)
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.setSession(sid)
	}
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil // 通知类请求
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("MCP 服务器返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if strings.Contains(ct, "text/event-stream") {
		return parseSSEBody(resp.Body, wantID)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return decodeRPC(b, wantID)
}

// parseSSEBody 从 SSE 响应体中提取与 wantID 匹配的 JSON-RPC 响应。
func parseSSEBody(r io.Reader, wantID int) (json.RawMessage, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var dataLines []string
	flush := func() (json.RawMessage, bool, error) {
		if len(dataLines) == 0 {
			return nil, false, nil
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil
		if payload == "" || payload == "[DONE]" {
			return nil, false, nil
		}
		var rp rpcResponse
		if err := json.Unmarshal([]byte(payload), &rp); err != nil {
			return nil, false, nil
		}
		if rp.ID == wantID || (wantID == 0 && rp.ID == 0) {
			if rp.Error != nil {
				return nil, true, fmt.Errorf("MCP 错误 %d: %s", rp.Error.Code, rp.Error.Message)
			}
			return rp.Result, true, nil
		}
		return nil, false, nil
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			res, done, err := flush()
			if err != nil {
				return nil, err
			}
			if done {
				return res, nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	// 流结束（无空行结尾）时再尝试一次
	res, done, err := flush()
	if err != nil {
		return nil, err
	}
	if done {
		return res, nil
	}
	return nil, errors.New("未在 SSE 流中找到响应")
}

// ---------- SSE 传输 ----------

func (c *Client) startSSE(ctx context.Context) error {
	c.sseMu.Lock()
	if c.sseReady != nil {
		c.sseMu.Unlock()
		return nil
	}
	c.sseReady = make(chan struct{})
	c.sseMu.Unlock()

	streamCtx, cancel := context.WithCancel(context.Background())
	c.sseCancel = cancel
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		cancel()
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	resp, err := c.sseHTTP.Do(req)
	if err != nil {
		cancel()
		return fmt.Errorf("SSE 连接失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		cancel()
		return fmt.Errorf("SSE 连接返回 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	go c.sseReadLoop(streamCtx, resp.Body)
	return nil
}

// postNotification 以 SSE 传输发送通知（忽略结果）。
func (c *Client) postNotification(ctx context.Context, body []byte) {
	c.sseMu.Lock()
	ready := c.sseReady
	c.sseMu.Unlock()
	if ready != nil {
		select {
		case <-ready:
		case <-time.After(10 * time.Second):
			return
		case <-ctx.Done():
			return
		}
	}
	c.sseMu.Lock()
	endpoint := c.sseEndpoint
	c.sseMu.Unlock()
	if endpoint == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	resp, err := c.sseHTTP.Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	resp.Body.Close()
}

func (c *Client) sseReadLoop(ctx context.Context, body io.ReadCloser) {
	defer body.Close()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var event, data string
	deliver := func() {
		if event == "endpoint" {
			endpoint := data
			if u, err := url.Parse(endpoint); err == nil && !u.IsAbs() {
				base, err := url.Parse(c.baseURL)
				if err == nil {
					endpoint = base.ResolveReference(u).String()
				}
			}
			c.sseMu.Lock()
			c.sseEndpoint = endpoint
			c.sseMu.Unlock()
			select {
			case <-c.sseReady:
			default:
				close(c.sseReady)
			}
		} else if event == "message" || event == "" {
			if data == "" || data == "[DONE]" {
				return
			}
			var rp rpcResponse
			if err := json.Unmarshal([]byte(data), &rp); err != nil {
				return
			}
			c.sseMu.Lock()
			ch, ok := c.ssePending[rp.ID]
			if !ok {
				// 响应先于等待者到达：缓存，等 callSSE 注册后取走
				c.sseEarly[rp.ID] = rp
			}
			c.sseMu.Unlock()
			if ok {
				select {
				case ch <- rp:
				default:
				}
			}
		}
		event, data = "", ""
	}
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			deliver()
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			d := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				data = d
			} else {
				data += "\n" + d
			}
		}
	}
	// 流断开：唤醒所有等待者
	c.sseMu.Lock()
	for id, ch := range c.ssePending {
		select {
		case ch <- rpcResponse{ID: id, Error: &rpcError{Code: -1, Message: "SSE 流已断开"}}:
		default:
		}
	}
	c.sseReady = nil
	c.sseMu.Unlock()
}

func (c *Client) callSSE(ctx context.Context, id int, body []byte) (json.RawMessage, error) {
	c.sseMu.Lock()
	ready := c.sseReady
	endpoint := c.sseEndpoint
	c.sseMu.Unlock()
	if ready != nil {
		select {
		case <-ready:
		case <-time.After(15 * time.Second):
			return nil, errors.New("等待 SSE endpoint 超时")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		c.sseMu.Lock()
		endpoint = c.sseEndpoint
		c.sseMu.Unlock()
	}
	if endpoint == "" {
		return nil, errors.New("SSE 未获得 endpoint")
	}
	ch := make(chan rpcResponse, 1)
	c.sseMu.Lock()
	c.ssePending[id] = ch
	// 检查是否已有提前到达的响应
	if early, ok := c.sseEarly[id]; ok {
		delete(c.sseEarly, id)
		c.sseMu.Unlock()
		ch <- early
	} else {
		c.sseMu.Unlock()
	}
	defer func() {
		c.sseMu.Lock()
		delete(c.ssePending, id)
		c.sseMu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SSE 消息发送失败: %w", err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("SSE 消息发送返回 HTTP %d", resp.StatusCode)
	}
	select {
	case rp := <-ch:
		if rp.Error != nil {
			return nil, fmt.Errorf("MCP 错误 %d: %s", rp.Error.Code, rp.Error.Message)
		}
		return rp.Result, nil
	case <-time.After(rpcTimeout):
		return nil, errors.New("等待 MCP 响应超时")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ---------- 高层 API ----------

// ListTools 返回服务器提供的工具列表。
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	res, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, fmt.Errorf("解析 tools/list 失败: %w", err)
	}
	return out.Tools, nil
}

// CallTool 调用指定工具，返回拼接后的文本结果。
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	res, err := c.call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(arguments),
	})
	if err != nil {
		return "", err
	}
	var tr toolCallResult
	if err := json.Unmarshal(res, &tr); err != nil {
		return "", fmt.Errorf("解析 tools/call 失败: %w", err)
	}
	var parts []string
	for _, c := range tr.Content {
		if c.Type == "text" || c.Type == "" {
			parts = append(parts, c.Text)
		} else {
			parts = append(parts, "["+c.Type+" 内容]")
		}
	}
	text := strings.Join(parts, "\n")
	if tr.IsError {
		return text, fmt.Errorf("工具返回错误: %s", truncate(text, 500))
	}
	return text, nil
}

// Close 关闭连接（SSE 模式下取消读取循环）。
func (c *Client) Close() {
	c.mu.Lock()
	cancel := c.sseCancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func decodeRPC(b []byte, wantID int) (json.RawMessage, error) {
	var rp rpcResponse
	if err := json.Unmarshal(b, &rp); err != nil {
		return nil, fmt.Errorf("解析 MCP 响应失败: %w", err)
	}
	if rp.Error != nil {
		return nil, fmt.Errorf("MCP 错误 %d: %s", rp.Error.Code, rp.Error.Message)
	}
	return rp.Result, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
