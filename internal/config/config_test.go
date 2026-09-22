package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// 文件不存在 → 默认值
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.BaseURL != "https://api.deepseek.com" || c.Model != "deepseek-chat" {
		t.Fatalf("默认值不符合预期: %+v", c)
	}

	c.APIKey = "sk-test"
	c.Workspace = `D:\work\proj`
	if err := c.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	c2, err := Load(path)
	if err != nil {
		t.Fatalf("Load2: %v", err)
	}
	if c2.APIKey != "sk-test" || c2.Workspace != `D:\work\proj` {
		t.Fatalf("往返失败: %+v", c2)
	}
}

func TestLoadMCPFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	content := `{
	  "mcpServers": {
	    "dianjian": { "url": "http://192.168.1.10:3000/mcp", "headers": {"Authorization": "Bearer x"} },
	    "sse-one":  { "type": "sse", "url": "http://192.168.1.10:3000/sse" },
	    "local":    { "command": "npx", "args": ["-y", "some-mcp"] }
	  }
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadMCPFile(path)
	if err != nil {
		t.Fatalf("LoadMCPFile: %v", err)
	}
	if len(f.MCPServers) != 3 {
		t.Fatalf("应有 3 个服务器，得到 %d", len(f.MCPServers))
	}
	dj := f.MCPServers["dianjian"]
	if dj.URL != "http://192.168.1.10:3000/mcp" || dj.Headers["Authorization"] != "Bearer x" {
		t.Fatalf("解析 dianjian 失败: %+v", dj)
	}
}

func TestLoadWithBOM(t *testing.T) {
	dir := t.TempDir()
	// 记事本 / PowerShell 5.1 保存的 UTF-8 带 BOM
	path := filepath.Join(dir, "mcp.json")
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"mcpServers":{"a":{"url":"http://h/mcp"}}}`)...)
	if err := os.WriteFile(path, withBOM, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := LoadMCPFile(path)
	if err != nil {
		t.Fatalf("带 BOM 的 mcp.json 应能解析: %v", err)
	}
	if len(f.MCPServers) != 1 {
		t.Fatalf("解析异常: %+v", f)
	}

	cfgPath := filepath.Join(dir, "config.json")
	withBOM2 := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"api_key":"sk-x","model":"m"}`)...)
	os.WriteFile(cfgPath, withBOM2, 0o644)
	c, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("带 BOM 的 config.json 应能解析: %v", err)
	}
	if c.APIKey != "sk-x" {
		t.Fatalf("解析异常: %+v", c)
	}
}
