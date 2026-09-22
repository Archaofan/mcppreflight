package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcpcheck/internal/config"
	"mcpcheck/internal/mcp"
)

// newFakeMCP 返回一个模拟 MCP Streamable HTTP 服务器（scan_project 工具）。
func newFakeMCP(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		switch req.Method {
		case "initialize":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":"2025-06-18","capabilities":{},"serverInfo":{"name":"fake","version":"1"}}}`, req.ID)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[{"name":"scan_project","description":"点检目录","inputSchema":{"type":"object","properties":{"path":{"type":"string"}}}}]}}`, req.ID)
		case "tools/call":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"content":[{"type":"text","text":"点检完成：3 个问题"}]}}`, req.ID)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSessionFullFlow 验证完整链路：
// 用户消息 → LLM 请求工具 → MCP 执行 → LLM 输出报告。
func TestSessionFullFlow(t *testing.T) {
	srv := newFakeMCP(t)
	mcpPath := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"dianjian":{"url":"`+srv.URL+`/mcp"}}}`), 0o644)

	ds := newFakeLLM(t)
	defer ds.Close()

	cfg := &config.Config{
		BaseURL:   ds.URL,
		APIKey:    "sk-test",
		Model:     "deepseek-chat",
		Workspace: `E:\DSH-Workspace\MCP-tool`,
		MCPConfig: mcpPath,
	}
	m := mcp.NewManager()
	if err := m.Load(context.Background(), mcpPath); err != nil {
		t.Fatalf("MCP Load: %v", err)
	}
	sess := New(cfg, m)

	var events []Event
	err := sess.Send(context.Background(), "请对工作区执行点检", func(ev Event) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	var hasCall, hasResult bool
	var content strings.Builder
	for _, ev := range events {
		switch ev.Type {
		case "tool_call":
			hasCall = true
			if ev.Name != "scan_project" {
				t.Errorf("工具名异常: %s", ev.Name)
			}
		case "tool_result":
			hasResult = true
		case "assistant_delta":
			content.WriteString(ev.Text)
		case "error":
			t.Errorf("不应出现错误事件: %s", ev.Text)
		}
	}
	if !hasCall || !hasResult {
		t.Fatalf("缺少工具调用事件: call=%v result=%v", hasCall, hasResult)
	}
	if !strings.Contains(content.String(), "报告") {
		t.Fatalf("最终输出应包含报告内容: %q", content.String())
	}
	// 历史应记录 user / assistant(tool_calls) / tool / assistant
	sess.mu.Lock()
	n := len(sess.history)
	sess.mu.Unlock()
	if n != 4 {
		t.Fatalf("历史应有 4 条消息，得到 %d", n)
	}
}

func TestSessionNoAPIKey(t *testing.T) {
	cfg := &config.Config{Workspace: "x"}
	m := mcp.NewManager()
	sess := New(cfg, m)
	var gotErr bool
	_ = sess.Send(context.Background(), "hi", func(ev Event) {
		if ev.Type == "error" {
			gotErr = true
		}
	})
	if !gotErr {
		t.Fatal("未配置 Key 时应报错")
	}
}

func TestSessionNoMCP(t *testing.T) {
	ds := newFakeLLM(t)
	defer ds.Close()
	cfg := &config.Config{BaseURL: ds.URL, APIKey: "sk-test", Model: "deepseek-chat", Workspace: "x"}
	sess := New(cfg, mcp.NewManager())
	var gotErr bool
	_ = sess.Send(context.Background(), "hi", func(ev Event) {
		if ev.Type == "error" {
			gotErr = true
		}
	})
	if !gotErr {
		t.Fatal("没有 MCP 连接时应报错")
	}
}

func TestSystemPromptIncludesWorkspace(t *testing.T) {
	cfg := &config.Config{Workspace: `D:\proj`}
	sess := New(cfg, mcp.NewManager())
	if !strings.Contains(sess.SystemPrompt(), `D:\proj`) {
		t.Fatal("系统提示应包含工作区路径")
	}
}

func TestReset(t *testing.T) {
	srv := newFakeMCP(t)
	mcpPath := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"d":{"url":"`+srv.URL+`/mcp"}}}`), 0o644)
	ds := newFakeLLM(t)
	defer ds.Close()
	cfg := &config.Config{BaseURL: ds.URL, APIKey: "k", Model: "m", Workspace: "w", MCPConfig: mcpPath}
	m := mcp.NewManager()
	m.Load(context.Background(), mcpPath)
	sess := New(cfg, m)
	sess.Send(context.Background(), "hi", func(ev Event) {})
	sess.Reset()
	sess.mu.Lock()
	n := len(sess.history)
	sess.mu.Unlock()
	if n != 0 {
		t.Fatalf("Reset 后历史应为空，得到 %d", n)
	}
}
