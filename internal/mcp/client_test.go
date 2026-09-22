package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// newFakeMCPServer 返回一个模拟 MCP Streamable HTTP 服务器。
// sse=true 时用 SSE 流返回响应，否则返回 JSON。
func newFakeMCPServer(t *testing.T, sse bool, requireSession bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		id := 0
		if req.ID != nil {
			id = *req.ID
		}
		if requireSession && req.Method != "initialize" && r.Header.Get("Mcp-Session-Id") == "" {
			http.Error(w, "missing session", 400)
			return
		}
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "sess-abc")
			writeResult(w, id, map[string]interface{}{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]string{"name": "fake", "version": "1.0"},
			}, sse)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeResult(w, id, map[string]interface{}{
				"tools": []Tool{{
					Name:        "scan_project",
					Description: "点检目录",
					InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
				}},
			}, sse)
		case "tools/call":
			writeResult(w, id, map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "点检完成：3 个问题"}},
			}, sse)
		default:
			http.Error(w, "unknown method", 400)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func writeResult(w http.ResponseWriter, id int, result interface{}, sse bool) {
	b, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result})
	if sse {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func TestClientStreamableJSON(t *testing.T) {
	srv := newFakeMCPServer(t, false, true)
	c := NewClient(srv.URL+"/mcp", nil, false)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if c.sessionID() != "sess-abc" {
		t.Fatalf("session id 未记录: %q", c.sessionID())
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "scan_project" {
		t.Fatalf("工具列表异常: %+v", tools)
	}
	res, err := c.CallTool(ctx, "scan_project", json.RawMessage(`{"path":"E:\\test"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !strings.Contains(res, "点检完成") {
		t.Fatalf("工具返回异常: %q", res)
	}
}

func TestClientStreamableSSEResponse(t *testing.T) {
	srv := newFakeMCPServer(t, true, false)
	c := NewClient(srv.URL+"/mcp", nil, false)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize(SSE): %v", err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools(SSE): %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("SSE 模式工具列表异常: %+v", tools)
	}
}

func TestClientRPCError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0", "id": 1,
			"error": map[string]interface{}{"code": -32601, "message": "method not found"},
		})
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL+"/mcp", nil, false)
	err := c.Initialize(context.Background())
	if err == nil || !strings.Contains(err.Error(), "method not found") {
		t.Fatalf("应返回 RPC 错误，得到: %v", err)
	}
}

func TestClientHeaders(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		writeResult(w, 1, map[string]interface{}{"protocolVersion": "2025-06-18"}, false)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClient(srv.URL+"/mcp", map[string]string{"Authorization": "Bearer tk"}, false)
	if err := c.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if gotAuth != "Bearer tk" {
		t.Fatalf("自定义 header 未发送: %q", gotAuth)
	}
}

// TestClientSSETransport 测试 SSE 传输：GET 建立流拿 endpoint，POST 发消息，流上收响应。
// 服务器按请求方法动态生成响应（与真实 MCP SSE 服务器行为一致）。
func TestClientSSETransport(t *testing.T) {
	type sseHub struct {
		mu      sync.Mutex
		clients map[chan string]struct{}
	}
	hub := &sseHub{clients: map[chan string]struct{}{}}
	subscribe := func() chan string {
		ch := make(chan string, 16)
		hub.mu.Lock()
		hub.clients[ch] = struct{}{}
		hub.mu.Unlock()
		return ch
	}
	unsubscribe := func(ch chan string) {
		hub.mu.Lock()
		delete(hub.clients, ch)
		hub.mu.Unlock()
	}
	broadcast := func(s string) {
		hub.mu.Lock()
		defer hub.mu.Unlock()
		for ch := range hub.clients {
			select {
			case ch <- s:
			default:
			}
		}
	}

	mux := http.NewServeMux()
	// SSE 流端点
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "event: endpoint\ndata: /messages?sessionId=xyz\n\n")
		flusher.Flush()
		ch := subscribe()
		defer unsubscribe(ch)
		for {
			select {
			case msg := <-ch:
				fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
				flusher.Flush()
			case <-r.Context().Done():
				return
			case <-time.After(15 * time.Second):
				return
			}
		}
	})
	// 消息发送端点：按方法动态生成响应并广播到 SSE 流
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if strings.HasPrefix(req.Method, "notifications/") {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		id := 0
		if req.ID != nil {
			id = *req.ID
		}
		var result interface{}
		switch req.Method {
		case "initialize":
			result = map[string]interface{}{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]interface{}{},
				"serverInfo":      map[string]string{"name": "sse-fake", "version": "1"},
			}
		case "tools/list":
			result = map[string]interface{}{
				"tools": []Tool{{Name: "scan_project", Description: "点检", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			}
		case "tools/call":
			result = map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": "SSE 点检完成"}},
			}
		default:
			result = nil
		}
		b, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": result})
		broadcast(string(b))
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(srv.URL+"/sse", nil, true)
	ctx := context.Background()
	if err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize(SSE transport): %v", err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools(SSE transport): %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "scan_project" {
		t.Fatalf("SSE 工具列表异常: %+v", tools)
	}
	res, err := c.CallTool(ctx, "scan_project", json.RawMessage(`{"path":"E:\\x"}`))
	if err != nil {
		t.Fatalf("CallTool(SSE transport): %v", err)
	}
	if !strings.Contains(res, "SSE 点检完成") {
		t.Fatalf("SSE 工具返回异常: %q", res)
	}
	c.Close()
}
