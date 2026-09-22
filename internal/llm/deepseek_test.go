package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mcpcheck/internal/mcp"
)

// fakeDeepSeek 模拟 DeepSeek：第一轮返回 tool_call（SSE 增量），
// 出现 tool 消息后返回流式文本报告。
func fakeDeepSeek(t *testing.T, seenTools *int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(w, "unauthorized", 401)
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
			Tools []map[string]interface{} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		*seenTools = len(body.Tools)
		hasToolMsg := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				hasToolMsg = true
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		send := func(v interface{}) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
		if !hasToolMsg {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{
				"tool_calls": []map[string]interface{}{{"index": 0, "id": "call_1", "type": "function",
					"function": map[string]string{"name": "scan_project", "arguments": `{"path"`}}},
			}}}})
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{
				"tool_calls": []map[string]interface{}{{"index": 0, "function": map[string]string{"arguments": `:"E:\\test"}`}}},
			}}}})
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{}, "finish_reason": "tool_calls"}}})
		} else {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "# 报告\n"}}}})
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "发现 **3** 个问题"}}}})
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"id":"deepseek-chat"},{"id":"deepseek-flash"}]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestChatStreamToolCallRound(t *testing.T) {
	seen := 0
	srv := fakeDeepSeek(t, &seen)
	c := NewClient(srv.URL, "sk-test", "deepseek-chat")
	msgs := []Message{{Role: "user", Content: "请点检"}}
	tools := []mcp.Tool{{Name: "scan_project", Description: "点检", InputSchema: json.RawMessage(`{"type":"object"}`)}}

	var deltas strings.Builder
	resp, err := c.ChatStream(context.Background(), msgs, tools, func(d string) { deltas.WriteString(d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if seen != 1 {
		t.Fatalf("应发送 1 个工具定义，得到 %d", seen)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("应解析出 1 个 tool_call: %+v", resp.ToolCalls)
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "scan_project" {
		t.Fatalf("tool_call 解析异常: %+v", tc)
	}
	if tc.Function.Arguments != `{"path":"E:\\test"}` {
		t.Fatalf("参数增量拼接错误: %q", tc.Function.Arguments)
	}
}

func TestChatStreamTextRound(t *testing.T) {
	seen := 0
	srv := fakeDeepSeek(t, &seen)
	c := NewClient(srv.URL, "sk-test", "deepseek-chat")
	msgs := []Message{
		{Role: "user", Content: "请点检"},
		{Role: "assistant", Content: "", ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: ToolCallFunction{Name: "scan_project", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: "点检完成"},
	}
	var deltas strings.Builder
	resp, err := c.ChatStream(context.Background(), msgs, nil, func(d string) { deltas.WriteString(d) })
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(resp.ToolCalls) != 0 {
		t.Fatalf("第二轮不应有 tool_call: %+v", resp.ToolCalls)
	}
	if resp.Content != "# 报告\n发现 **3** 个问题" {
		t.Fatalf("内容拼接错误: %q", resp.Content)
	}
}

func TestListModels(t *testing.T) {
	seen := 0
	srv := fakeDeepSeek(t, &seen)
	c := NewClient(srv.URL, "sk-test", "deepseek-chat")
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 || models[0] != "deepseek-chat" {
		t.Fatalf("模型列表异常: %v", models)
	}
}

func TestChatStreamAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid api key"}}`, 401)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "bad", "m")
	_, err := c.ChatStream(context.Background(), []Message{{Role: "user", Content: "x"}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("应透出 API 错误信息，得到: %v", err)
	}
}

func TestBuildTools(t *testing.T) {
	tools := []mcp.Tool{{Name: "a", Description: "d"}}
	built := BuildTools(tools)
	if len(built) != 1 || built[0].Type != "function" || built[0].Function.Name != "a" {
		t.Fatalf("BuildTools 异常: %+v", built)
	}
	if string(built[0].Function.Parameters) != `{"type":"object","properties":{}}` {
		t.Fatalf("空 schema 应补默认值: %s", built[0].Function.Parameters)
	}
}
