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
	"sync"
	"testing"

	"mcppreflight/internal/chat"
	"mcppreflight/internal/config"
	"mcppreflight/internal/mcp"
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
	llmSrv := fakeLLM(t)
	return newTestServerWithLLM(t, llmSrv.URL)
}

func newTestServerWithLLM(t *testing.T, llmURL string) (*Server, *httptest.Server) {
	t.Helper()
	mcpSrv := fakeMCP(t)
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, "mcp.json")
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"dianjian":{"url":"`+mcpSrv.URL+`/mcp"}}}`), 0o644)
	cfgPath := filepath.Join(dir, "config.json")

	cfg := &config.Config{
		Provider:  "deepseek",
		BaseURL:   llmURL,
		APIKey:    "sk-test",
		Model:     "deepseek-flash",
		Workspace: dir,
		MCPConfig: mcpPath,
		Lang:      "zh-CN",
		Theme:     "light",
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

// /api/providers 应返回全部内置服务商预设（界面下拉的唯一数据源）
func TestProvidersEndpoint(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		OK        bool `json:"ok"`
		Providers []struct {
			ID      string   `json:"id"`
			Name    string   `json:"name"`
			NameEN  string   `json:"name_en"`
			BaseURL string   `json:"base_url"`
			Models  []string `json:"models"`
			Tools   string   `json:"tools"`
		} `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || len(got.Providers) < 10 {
		t.Fatalf("服务商列表异常: %+v", got)
	}
	if got.Providers[0].ID != "deepseek" {
		t.Fatalf("第一个服务商应为 deepseek，实际 %s", got.Providers[0].ID)
	}
	var customSeen bool
	for _, p := range got.Providers {
		if p.ID == "custom" {
			customSeen = true
			continue
		}
		if p.BaseURL == "" || p.Name == "" || p.NameEN == "" {
			t.Fatalf("服务商 %s 字段不完整: %+v", p.ID, p)
		}
		if p.Tools != "full" && p.Tools != "partial" && p.Tools != "none" {
			t.Fatalf("服务商 %s tools 字段异常: %s", p.ID, p.Tools)
		}
	}
	if !customSeen {
		t.Fatal("应包含 custom 占位")
	}
	// DeepSeek 的模型清单必须是现行 ID
	for _, m := range got.Providers[0].Models {
		if m == "deepseek-chat" || m == "deepseek-reasoner" {
			t.Fatalf("DeepSeek 不应再返回已停用模型 %s", m)
		}
	}
}

// 语言 / 主题 / 服务商 三个新配置项应能保存、读取，并对非法值保持原值
func TestConfigLangThemeProvider(t *testing.T) {
	_, ts := newTestServer(t)
	post := func(body string) map[string]interface{} {
		resp, err := http.Post(ts.URL+"/api/config", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&m)
		return m
	}
	if r := post(`{"lang":"en","theme":"dark","provider":"glm","model":"glm-5.3"}`); r["ok"] != true {
		t.Fatalf("保存应成功: %v", r)
	}
	resp, _ := http.Get(ts.URL + "/api/config")
	var got map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got["lang"] != "en" || got["theme"] != "dark" || got["provider"] != "glm" {
		t.Fatalf("新字段保存/读取异常: %v", got)
	}
	// 非法值不应覆盖已有配置
	post(`{"theme":"blue","provider":"not-exist","lang":"fr"}`)
	resp2, _ := http.Get(ts.URL + "/api/config")
	var got2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&got2)
	resp2.Body.Close()
	if got2["theme"] != "dark" || got2["provider"] != "glm" || got2["lang"] != "zh-CN" {
		t.Fatalf("非法值应被忽略: %v", got2)
	}
}

// 已停用的模型 ID 在保存时应自动迁移（保证 DeepSeek 可用）
func TestConfigModelMigration(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/api/config", "application/json",
		strings.NewReader(`{"model":"deepseek-chat"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp2, _ := http.Get(ts.URL + "/api/config")
	var got map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&got)
	resp2.Body.Close()
	if got["model"] != "deepseek-flash" {
		t.Fatalf("deepseek-chat 应被迁移为 deepseek-flash，实际 %v", got["model"])
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

// 带 tools 的多轮对话必须回传 reasoning_content（DeepSeek/Kimi/硅基流动的硬性要求），
// 且每一轮都要重复携带 tools 与 tool_choice=auto。
func TestReasoningContentRoundTrip(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		mu.Lock()
		bodies = append(bodies, m)
		n := len(bodies)
		mu.Unlock()

		hasToolResult := false
		if msgs, ok := m["messages"].([]interface{}); ok {
			for _, mm := range msgs {
				if msg, ok := mm.(map[string]interface{}); ok && msg["role"] == "tool" {
					hasToolResult = true
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		send := func(v interface{}) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
		if !hasToolResult {
			// 第一轮：思考内容 + 工具调用
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{
				"reasoning_content": "先扫描工作区再下结论",
				"tool_calls": []map[string]interface{}{{"index": 0, "id": "call_r", "type": "function",
					"function": map[string]string{"name": "scan_project", "arguments": `{"path":"E:\\test"}`}}},
			}}}})
		} else {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "# 点检报告\n一切正常"}}}})
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
		_ = n
	}))
	defer llm.Close()

	_, ts := newTestServerWithLLM(t, llm.URL)
	resp, err := http.Post(ts.URL+"/api/chat", "application/json", strings.NewReader(`{"message":"点检一下"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var ev chat.Event
		json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &ev)
		if ev.Type == "error" {
			t.Fatalf("聊天流程出错: %s", ev.Text)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) < 2 {
		t.Fatalf("应有两轮请求，实际 %d 轮", len(bodies))
	}
	for i, b := range bodies {
		tools, _ := b["tools"].([]interface{})
		if len(tools) == 0 {
			t.Fatalf("第 %d 轮请求未携带 tools", i+1)
		}
		if b["tool_choice"] != "auto" {
			t.Fatalf("第 %d 轮请求 tool_choice 应为 auto，实际 %v", i+1, b["tool_choice"])
		}
	}
	// 第二轮（带 tools 的后续请求）必须回传首轮的 reasoning_content
	msgs, _ := bodies[1]["messages"].([]interface{})
	var found bool
	for _, mm := range msgs {
		msg, _ := mm.(map[string]interface{})
		if msg["role"] == "assistant" && msg["reasoning_content"] == "先扫描工作区再下结论" {
			found = true
		}
	}
	if !found {
		t.Fatalf("第二轮请求未回传 reasoning_content: %v", msgs)
	}
}

// 厂商私有参数必须进入请求体（星火 tool_calls_switch / 硅基流动 enable_thinking）
func TestProviderExtraBodySent(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]interface{}
		json.Unmarshal(raw, &m)
		mu.Lock()
		bodies = append(bodies, m)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		b, _ := json.Marshal(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "ok"}}}})
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer llm.Close()

	s, ts := newTestServerWithLLM(t, llm.URL)
	s.Cfg.Provider = "spark"
	// 直接走会话层，避免 HTTP 层把 provider 覆盖回 deepseek
	m := s.MCP
	sess := chat.New(s.Cfg, m)
	s.Session = sess

	resp, err := http.Post(ts.URL+"/api/chat", "application/json", strings.NewReader(`{"message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) == 0 {
		t.Fatal("没有收到请求")
	}
	if bodies[0]["tool_calls_switch"] != true {
		t.Fatalf("星火请求应包含 tool_calls_switch=true: %v", bodies[0])
	}
}

// Ollama：空 Key 可用，且不下发 tool_choice
func TestOllamaDegradation(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]interface{}
	var authSeen string
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var m map[string]interface{}
		json.Unmarshal(raw, &m)
		mu.Lock()
		bodies = append(bodies, m)
		authSeen = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		b, _ := json.Marshal(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "ok"}}}})
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer llm.Close()

	s, ts := newTestServerWithLLM(t, llm.URL)
	s.Cfg.Provider = "ollama"
	s.Cfg.APIKey = ""
	s.Session = chat.New(s.Cfg, s.MCP)

	resp, err := http.Post(ts.URL+"/api/chat", "application/json", strings.NewReader(`{"message":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) == 0 {
		t.Fatal("没有收到请求")
	}
	if _, ok := bodies[0]["tool_choice"]; ok {
		t.Fatalf("Ollama 不应下发 tool_choice: %v", bodies[0])
	}
	if authSeen == "" || authSeen == "Bearer " {
		t.Fatalf("空 Key 应补占位符，实际 %q", authSeen)
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
