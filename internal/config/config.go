package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是应用的本地配置（保存在 exe 同目录的 config.json）。
type Config struct {
	BaseURL   string `json:"base_url"`   // OpenAI 兼容 API 地址，默认 https://api.deepseek.com
	APIKey    string `json:"api_key"`    // API Key（明文保存在本机 config.json）
	Model     string `json:"model"`      // 模型名，如 deepseek-chat / deepseek-flash
	Workspace string `json:"workspace"`  // 工作区文件夹（点检目标目录）
	MCPConfig string `json:"mcp_config"` // mcp.json 路径（cursor 格式）
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		BaseURL: "https://api.deepseek.com",
		Model:   "deepseek-chat",
	}
}

// Load 从 path 读取配置；文件不存在时返回默认配置。
func Load(path string) (*Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.deepseek.com"
	}
	if c.Model == "" {
		c.Model = "deepseek-chat"
	}
	return c, nil
}

// Save 保存配置到 path（权限 0600，仅当前用户可读写）。
func (c *Config) Save(path string) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// MCPServerEntry 是 mcp.json 中单个服务器的定义（cursor 格式）。
type MCPServerEntry struct {
	Type    string            `json:"type,omitempty"`    // streamableHttp / http / sse；缺省按 URL 推断
	URL     string            `json:"url,omitempty"`     // 远程地址
	Headers map[string]string `json:"headers,omitempty"` // 自定义请求头（如 Authorization）
	Command string            `json:"command,omitempty"` // stdio 启动命令（本版本暂不支持）
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPFile 是 mcp.json 的结构。
type MCPFile struct {
	MCPServers map[string]MCPServerEntry `json:"mcpServers"`
}

// LoadMCPFile 读取并解析 mcp.json。
func LoadMCPFile(path string) (*MCPFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f MCPFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// ExeDir 返回可执行文件所在目录（配置文件默认放在这里）。
func ExeDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}
