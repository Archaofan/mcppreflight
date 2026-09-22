package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mcpcheck/internal/config"
)

// ServerState 是一个 MCP 服务器的运行时状态（供 UI 展示）。
type ServerState struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Type   string `json:"type"`   // streamable / sse / stdio
	Status string `json:"status"` // ok / error / unsupported
	Error  string `json:"error,omitempty"`
	Tools  []Tool `json:"tools"`
}

// Manager 管理 mcp.json 中定义的所有 MCP 服务器连接。
type Manager struct {
	mu      sync.Mutex
	servers map[string]*ServerState
	clients map[string]*Client
	path    string
}

// NewManager 创建空的管理器。
func NewManager() *Manager {
	return &Manager{
		servers: map[string]*ServerState{},
		clients: map[string]*Client{},
	}
}

// Path 返回当前加载的 mcp.json 路径。
func (m *Manager) Path() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.path
}

// Load 读取 mcp.json 并（重新）连接所有服务器。已连接的旧连接会被关闭。
func (m *Manager) Load(ctx context.Context, path string) error {
	m.mu.Lock()
	for _, c := range m.clients {
		c.Close()
	}
	m.servers = map[string]*ServerState{}
	m.clients = map[string]*Client{}
	m.path = path
	m.mu.Unlock()

	f, err := config.LoadMCPFile(path)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(f.MCPServers))
	for name := range f.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		entry := f.MCPServers[name]
		st := &ServerState{Name: name, URL: entry.URL}
		transport := DetectType(entry.Type, entry.URL, entry.Command)
		switch transport {
		case "stdio":
			st.Type = "stdio"
			st.Status = "unsupported"
			st.Error = "本版本暂不支持 stdio（本地命令）类型，请使用 http/sse 类型"
			m.mu.Lock()
			m.servers[name] = st
			m.mu.Unlock()
			continue
		case "sse":
			st.Type = "sse"
		default:
			st.Type = "streamable"
		}
		if entry.URL == "" {
			st.Status = "error"
			st.Error = "缺少 url 字段"
			m.mu.Lock()
			m.servers[name] = st
			m.mu.Unlock()
			continue
		}

		client := NewClient(entry.URL, entry.Headers, transport == "sse")
		connCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := client.Initialize(connCtx)
		cancel()
		if err != nil {
			st.Status = "error"
			st.Error = err.Error()
			m.mu.Lock()
			m.servers[name] = st
			m.mu.Unlock()
			continue
		}
		tools, err := client.ListTools(ctx)
		if err != nil {
			st.Status = "error"
			st.Error = "连接成功但获取工具列表失败: " + err.Error()
			m.mu.Lock()
			m.servers[name] = st
			m.mu.Unlock()
			continue
		}
		if tools == nil {
			tools = []Tool{}
		}
		st.Status = "ok"
		st.Tools = tools
		m.mu.Lock()
		m.servers[name] = st
		m.clients[name] = client
		m.mu.Unlock()
	}
	return nil
}

// Servers 返回所有服务器状态的副本（按名称排序）。
func (m *Manager) Servers() []ServerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ServerState, 0, len(m.servers))
	for _, st := range m.servers {
		cp := *st
		cp.Tools = append([]Tool{}, st.Tools...)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ToolWithServer 是带服务器归属的工具。
type ToolWithServer struct {
	Server string `json:"server"`
	Tool   Tool   `json:"tool"`
}

// AllTools 返回所有已连接服务器的工具；重名时加 "@服务器名" 后缀。
func (m *Manager) AllTools() []Tool {
	pairs := m.allToolPairs()
	// 统计重名
	count := map[string]int{}
	for _, p := range pairs {
		count[p.Tool.Name]++
	}
	out := make([]Tool, 0, len(pairs))
	for _, p := range pairs {
		t := p.Tool
		if count[t.Name] > 1 {
			t.Name = t.Name + "@" + p.Server
		}
		out = append(out, t)
	}
	return out
}

func (m *Manager) allToolPairs() []ToolWithServer {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []ToolWithServer
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		st := m.servers[name]
		if st.Status != "ok" {
			continue
		}
		for _, t := range st.Tools {
			out = append(out, ToolWithServer{Server: name, Tool: t})
		}
	}
	return out
}

// CallTool 按（可能带 @服务器 后缀的）工具名调用工具。
func (m *Manager) CallTool(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	server, realName := name, name
	if idx := strings.LastIndex(name, "@"); idx > 0 {
		server, realName = name[idx+1:], name[:idx]
	}
	client, ok := m.resolve(server, realName, name)
	if !ok || client == nil {
		return "", fmt.Errorf("找不到工具 %q 对应的 MCP 服务器", name)
	}
	return client.CallTool(ctx, realName, arguments)
}

// resolve 优先按服务器名查找，找不到时按工具名在全部已连接服务器里查找。
func (m *Manager) resolve(server, realName, fullName string) (*Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if client, ok := m.clients[server]; ok {
		return client, true
	}
	for sname, st := range m.servers {
		if st.Status != "ok" {
			continue
		}
		for _, t := range st.Tools {
			if t.Name == realName || t.Name == fullName {
				return m.clients[sname], true
			}
		}
	}
	return nil, false
}

// ConnectedCount 返回已连接服务器数量。
func (m *Manager) ConnectedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, st := range m.servers {
		if st.Status == "ok" {
			n++
		}
	}
	return n
}
