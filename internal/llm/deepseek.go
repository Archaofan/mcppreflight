// Package llm 实现 OpenAI 兼容的聊天客户端（默认适配 DeepSeek），
// 支持流式输出与 Function Calling（工具调用）。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mcppreflight/internal/mcp"
)

// Message 是一条对话消息（OpenAI 格式）。
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// ReasoningContent 是思考内容。DeepSeek / Kimi / 硅基流动等在带 tools 的
	// 多轮对话中要求原样回传，否则接口报错，因此必须作为一等字段存进历史。
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

// ToolCall 是模型发起的一次工具调用。
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 包含工具名与 JSON 参数。
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type chatRequest struct {
	Model      string       `json:"model"`
	Messages   []Message    `json:"messages"`
	Tools      []openAITool `json:"tools,omitempty"`
	ToolChoice string       `json:"tool_choice,omitempty"`
	Stream     bool         `json:"stream"`
	// Extra 是厂商私有参数（不进 JSON 结构体，序列化时合并进请求体）
	Extra map[string]interface{} `json:"-"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Client 是 OpenAI 兼容 API 客户端。
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	// Extra 是厂商私有附加参数（如星火 tool_calls_switch、硅基流动 enable_thinking）
	Extra map[string]interface{}
	// OmitToolChoice 为 true 时不下发 tool_choice（如 Ollama 不支持）
	OmitToolChoice bool
	HTTP           *http.Client
}

// NewClient 创建客户端。
func NewClient(baseURL, apiKey, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// WithExtra 设置厂商私有附加参数（链式调用）。
func (c *Client) WithExtra(extra map[string]interface{}) *Client {
	c.Extra = extra
	return c
}

func (c *Client) endpoint() string {
	return c.BaseURL + "/chat/completions"
}

// BuildTools 把 MCP 工具转换为 OpenAI tools 格式。
func BuildTools(mcpTools []mcp.Tool) []openAITool {
	if len(mcpTools) == 0 {
		return nil
	}
	out := make([]openAITool, 0, len(mcpTools))
	for _, t := range mcpTools {
		schema := t.InputSchema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, openAITool{
			Type: "function",
			Function: openAIFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schema,
			},
		})
	}
	return out
}

// ChatStream 发送一轮对话（流式）。onDelta 在收到内容增量时回调。
// 返回的 Message 包含完整的 content、reasoning_content 与 tool_calls（如有）。
func (c *Client) ChatStream(ctx context.Context, msgs []Message, mcpTools []mcp.Tool, onDelta func(string)) (*Message, error) {
	body := chatRequest{
		Model:    c.Model,
		Messages: msgs,
		Tools:    BuildTools(mcpTools),
		Stream:   true,
		Extra:    c.Extra,
	}
	if len(mcpTools) > 0 && !c.OmitToolChoice {
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	// 合并厂商私有参数（Extra 优先，不覆盖结构体字段）
	if len(c.Extra) > 0 {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(payload, &m); err == nil {
			for k, v := range c.Extra {
				b, err := json.Marshal(v)
				if err != nil {
					continue
				}
				m[k] = b
			}
			if merged, err := json.Marshal(m); err == nil {
				payload = merged
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	// 本地 Ollama 等服务允许空 Key（其忽略该值），此处补占位符避免被中间层拒绝
	key := c.APIKey
	if key == "" {
		key = "ollama"
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接模型 API 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("模型 API 返回 HTTP %d: %s", resp.StatusCode, extractAPIError(b))
	}

	out := &Message{Role: "assistant"}
	toolByIdx := map[int]*ToolCall{}
	var order []int

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var ch streamChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			continue // 忽略无法解析的心跳/注释行
		}
		if ch.Error != nil && ch.Error.Message != "" {
			return nil, fmt.Errorf("模型 API 错误: %s", ch.Error.Message)
		}
		for _, choice := range ch.Choices {
			d := choice.Delta
			if d.ReasoningContent != "" {
				out.ReasoningContent += d.ReasoningContent
			}
			if d.Content != "" {
				out.Content += d.Content
				if onDelta != nil {
					onDelta(d.Content)
				}
			}
			for _, tc := range d.ToolCalls {
				call, ok := toolByIdx[tc.Index]
				if !ok {
					call = &ToolCall{ID: tc.ID, Type: "function"}
					if call.ID == "" {
						call.ID = fmt.Sprintf("call_%d", tc.Index)
					}
					toolByIdx[tc.Index] = call
					order = append(order, tc.Index)
				}
				if tc.ID != "" && call.ID == "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" {
					call.Function.Name += tc.Function.Name
				}
				call.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	if err := sc.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("读取流式响应失败: %w", err)
	}
	for _, idx := range order {
		call := toolByIdx[idx]
		if call.Function.Arguments == "" {
			call.Function.Arguments = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, *call)
	}
	return out, nil
}

// ListModels 拉取模型列表（GET {base}/models）。
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, extractAPIError(b))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	var ids []string
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	return ids, nil
}

// extractAPIError 尽力从错误响应体中提取可读信息。
func extractAPIError(b []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(b, &e); err == nil {
		if e.Error.Message != "" {
			return e.Error.Message
		}
		if e.Message != "" {
			return e.Message
		}
	}
	s := strings.TrimSpace(string(b))
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return s
}
