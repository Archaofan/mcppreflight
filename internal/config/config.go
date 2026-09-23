package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config 是应用的本地配置（保存在 exe 同目录的 config.json）。
type Config struct {
	Provider  string `json:"provider"`   // 模型厂商预设 ID（deepseek / qwen / glm ...；custom = 自定义）
	BaseURL   string `json:"base_url"`   // OpenAI 兼容 API 地址，默认 https://api.deepseek.com
	APIKey    string `json:"api_key"`    // API Key（明文保存在本机 config.json）
	Model     string `json:"model"`      // 模型名，如 deepseek-chat
	Workspace string `json:"workspace"`  // 工作区文件夹（点检目标目录）
	MCPConfig string `json:"mcp_config"` // mcp.json 路径（cursor 格式）
	Lang      string `json:"lang"`       // 界面语言：zh-CN（默认）/ en
	Theme     string `json:"theme"`      // 界面主题：light（默认）/ dark
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		Provider: "deepseek",
		BaseURL:  "https://api.deepseek.com",
		Model:    "deepseek-flash",
		Lang:     "zh-CN",
		Theme:    "light",
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
	b = trimBOM(b)
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	// 兼容旧版配置：缺省字段按默认值补齐，不改变用户已有选择
	if c.Provider == "" {
		c.Provider = InferProvider(c.BaseURL)
	}
	if c.BaseURL == "" {
		c.BaseURL = "https://api.deepseek.com"
	}
	if c.Model == "" {
		c.Model = "deepseek-flash"
	}
	// 旧版配置里的已停用模型 ID 自动迁移（如 deepseek-chat → deepseek-flash）
	c.Model = MigrateModel(c.Model)
	if c.Lang == "" {
		c.Lang = "zh-CN"
	}
	if c.Theme == "" {
		c.Theme = "light"
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
	b = trimBOM(b)
	var f MCPFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// trimBOM 去掉 UTF-8 BOM（记事本/ PowerShell 5.1 保存的 JSON 常带 BOM）。
func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
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
