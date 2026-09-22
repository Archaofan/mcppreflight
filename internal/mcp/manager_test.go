package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeMCPJSON(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectType(t *testing.T) {
	cases := []struct {
		typ, url, cmd, want string
	}{
		{"", "http://h/mcp", "", "streamable"},
		{"http", "http://h/mcp", "", "streamable"},
		{"streamableHttp", "http://h/mcp", "", "streamable"},
		{"sse", "http://h/mcp", "", "sse"},
		{"", "http://h/sse", "", "sse"},
		{"", "", "npx", "stdio"},
	}
	for _, c := range cases {
		if got := DetectType(c.typ, c.url, c.cmd); got != c.want {
			t.Errorf("DetectType(%q,%q,%q)=%q, want %q", c.typ, c.url, c.cmd, got, c.want)
		}
	}
}

func TestManagerLoadAndRoute(t *testing.T) {
	srv := newFakeMCPServer(t, false, false)
	path := writeMCPJSON(t, `{
	  "mcpServers": {
	    "dianjian": {"url": "`+srv.URL+`/mcp"},
	    "local": {"command": "npx", "args": ["x"]}
	  }
	}`)
	m := NewManager()
	if err := m.Load(context.Background(), path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	servers := m.Servers()
	if len(servers) != 2 {
		t.Fatalf("应有 2 个服务器，得到 %d", len(servers))
	}
	byName := map[string]ServerState{}
	for _, s := range servers {
		byName[s.Name] = s
	}
	if byName["dianjian"].Status != "ok" {
		t.Fatalf("dianjian 应连接成功: %+v", byName["dianjian"])
	}
	if len(byName["dianjian"].Tools) != 1 {
		t.Fatalf("应有 1 个工具: %+v", byName["dianjian"].Tools)
	}
	if byName["local"].Status != "unsupported" {
		t.Fatalf("stdio 应标记 unsupported: %+v", byName["local"])
	}
	if m.ConnectedCount() != 1 {
		t.Fatalf("连接数应为 1，得到 %d", m.ConnectedCount())
	}

	tools := m.AllTools()
	if len(tools) != 1 || tools[0].Name != "scan_project" {
		t.Fatalf("AllTools 异常: %+v", tools)
	}
	res, err := m.CallTool(context.Background(), "scan_project", json.RawMessage(`{"path":"E:\\x"}`))
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res == "" {
		t.Fatal("CallTool 返回为空")
	}
}

func TestManagerLoadMissingFile(t *testing.T) {
	m := NewManager()
	err := m.Load(context.Background(), filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Fatal("加载不存在的文件应报错")
	}
}

func TestManagerReloadClosesOld(t *testing.T) {
	srv := newFakeMCPServer(t, false, false)
	path := writeMCPJSON(t, `{"mcpServers":{"a":{"url":"`+srv.URL+`/mcp"}}}`)
	m := NewManager()
	if err := m.Load(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	// 第二次加载应替换旧连接且不报错
	if err := m.Load(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if m.ConnectedCount() != 1 {
		t.Fatalf("重载后连接数异常: %d", m.ConnectedCount())
	}
}
