package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcpcheck/internal/chat"
	"mcpcheck/internal/config"
	"mcpcheck/internal/mcp"
)

// fakeMCP 模拟 MCP Streamable HTTP 服务器。
func fakeMCP(t *testing.T) *httptest.Server {
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

// fakeLLM 模拟 DeepSeek（首轮 tool_call，次轮流式报告）。
func fakeLLM(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		hasTool := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				hasTool = true
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		send := func(v interface{}) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
		if !hasTool {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{
				"tool_calls": []map[string]interface{}{{"index": 0, "id": "call_1", "type": "function",
					"function": map[string]string{"name": "scan_project", "arguments": `{"path":"E:\\test"}`}}},
			}}}})
		} else {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "# 点检报告\n一切正常"}}}})
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	mcpSrv := fakeMCP(t)
	llmSrv := fakeLLM(t)
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, "mcp.json")
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"dianjian":{"url":"`+mcpSrv.URL+`/mcp"}}}`), 0o644)
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		BaseURL:   llmSrv.URL,
		APIKey:    "sk-test",
		Model:     "deepseek-chat",
		Workspace: dir,
		MCPConfig: mcpPath,
	}
	m := mcp.NewManager()
	if err := m.Load(context.Background(), mcpPath); err != nil {
		t.Fatalf("MCP Load: %v", err)
	}
	sess := chat.New(cfg, m)
	s := New(cfg, cfgPath, m, sess)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func TestIndexPage(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "MCP点检助手") {
		t.Fatal("首页应包含应用标题")
	}
	if !strings.Contains(string(body), "app.js") {
		t.Fatal("首页应引用 app.js")
	}
}

func TestStaticAssets(t *testing.T) {
	_, ts := newTestServer(t)
	for _, path := range []string{"/app.js", "/style.css"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s 应返回 200，得到 %d", path, resp.StatusCode)
		}
	}
}

func TestConfigRoundTrip(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["api_key"] != "sk-test" {
		t.Fatalf("配置读取异常: %v", got)
	}

	resp2, err := http.Post(ts.URL+"/api/config", "application/json",
		strings.NewReader(`{"base_url":"https://api.deepseek.com","api_key":"sk-new","model":"deepseek-flash"}`))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&saved)
	resp2.Body.Close()
	if saved["ok"] != true {
		t.Fatalf("保存配置应成功: %v", saved)
	}
	resp3, _ := http.Get(ts.URL + "/api/config")
	var got2 map[string]interface{}
	json.NewDecoder(resp3.Body).Decode(&got2)
	resp3.Body.Close()
	if got2["api_key"] != "sk-new" || got2["model"] != "deepseek-flash" {
		t.Fatalf("保存后读取异常: %v", got2)
	}
}

func TestServersEndpoint(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/servers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		OK      bool `json:"ok"`
		Servers []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Tools  []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"servers"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	if len(got.Servers) != 1 || got.Servers[0].Status != "ok" {
		t.Fatalf("服务器状态异常: %+v", got.Servers)
	}
	if len(got.Servers[0].Tools) != 1 || got.Servers[0].Tools[0].Name != "scan_project" {
		t.Fatalf("工具列表异常: %+v", got.Servers[0].Tools)
	}
}

func TestChatSSEFlow(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/chat", "application/json",
		strings.NewReader(`{"message":"请点检工作区"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("Content-Type 应为 event-stream，得到 %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	var types []string
	var content string
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev chat.Event
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev); err != nil {
			t.Fatalf("事件解析失败: %v (%s)", err, line)
		}
		types = append(types, ev.Type)
		if ev.Type == "assistant_delta" {
			content += ev.Text
		}
		if ev.Type == "error" {
			t.Fatalf("聊天流程出现错误: %s", ev.Text)
		}
	}
	joined := strings.Join(types, ",")
	for _, want := range []string{"tool_call", "tool_result", "assistant_delta", "done"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("事件序列缺少 %s: %s", want, joined)
		}
	}
	if !strings.Contains(content, "点检报告") {
		t.Fatalf("最终内容异常: %q", content)
	}
}

func TestResetEndpoint(t *testing.T) {
	s, ts := newTestServer(t)
	// 先跑一轮产生历史
	http.Post(ts.URL+"/api/chat", "application/json", strings.NewReader(`{"message":"hi"}`))
	resp, err := http.Post(ts.URL+"/api/reset", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset 应返回 200，得到 %d", resp.StatusCode)
	}
	_ = s
}
