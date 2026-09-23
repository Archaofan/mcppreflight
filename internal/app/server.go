// Package app 提供本地 HTTP 服务：REST API + SSE 对话流 + 嵌入式 Web UI。
package app

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"mcppreflight/internal/chat"
	"mcppreflight/internal/config"
	"mcppreflight/internal/i18n"
	"mcppreflight/internal/llm"
	"mcppreflight/internal/mcp"
)

//go:embed assets
var assetsFS embed.FS

// Server 是本地 API 服务器。
type Server struct {
	Cfg     *config.Config
	CfgPath string
	MCP     *mcp.Manager
	Session *chat.Session

	Addr   string
	quitFn func()

	mu      sync.Mutex
	started bool
}

// New 创建服务器。
func New(cfg *config.Config, cfgPath string, m *mcp.Manager, sess *chat.Session) *Server {
	return &Server{Cfg: cfg, CfgPath: cfgPath, MCP: m, Session: sess}
}

// Handler 返回注册好全部路由的 http.Handler（便于测试）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		fileServer.ServeHTTP(w, r)
	})

	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/providers", s.handleProviders)
	mux.HandleFunc("/api/servers", s.handleServers)
	mux.HandleFunc("/api/servers/reload", s.handleServersReload)
	mux.HandleFunc("/api/pick-folder", s.handlePickFolder)
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/reset", s.handleReset)
	mux.HandleFunc("/api/quit", s.handleQuit)
	return mux
}

// Start 在 127.0.0.1 上寻找可用端口并启动服务（非阻塞）。
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	addr := s.Addr
	if addr == "" {
		addr = "127.0.0.1:0" // 0 = 由系统自动选择空闲端口
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.Addr = ln.Addr().String()
	srv := &http.Server{Handler: s.Handler()}
	s.started = true
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP 服务退出: %v", err)
		}
	}()
	return nil
}

// URL 返回本服务的访问地址。
func (s *Server) URL() string {
	return "http://" + s.Addr
}

/* ----------------  handlers  ---------------- */

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{
			"ok":         true,
			"provider":   s.Cfg.Provider,
			"base_url":   s.Cfg.BaseURL,
			"api_key":    s.Cfg.APIKey,
			"model":      s.Cfg.Model,
			"workspace":  s.Cfg.Workspace,
			"mcp_config": s.Cfg.MCPConfig,
			"lang":       s.Cfg.Lang,
			"theme":      s.Cfg.Theme,
		})
	case http.MethodPost:
		var body struct {
			Provider  string `json:"provider"`
			BaseURL   string `json:"base_url"`
			APIKey    string `json:"api_key"`
			Model     string `json:"model"`
			MCPConfig string `json:"mcp_config"`
			Lang      string `json:"lang"`
			Theme     string `json:"theme"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "error": i18n.T(s.lang(), "invalid_json_body")})
			return
		}
		if p := strings.TrimSpace(body.Provider); p != "" && config.ProviderByID(p) != nil {
			s.Cfg.Provider = p
		}
		if strings.TrimSpace(body.BaseURL) != "" {
			s.Cfg.BaseURL = strings.TrimRight(strings.TrimSpace(body.BaseURL), "/")
		}
		if strings.TrimSpace(body.APIKey) != "" {
			s.Cfg.APIKey = strings.TrimSpace(body.APIKey)
		}
		if strings.TrimSpace(body.Model) != "" {
			s.Cfg.Model = config.MigrateModel(strings.TrimSpace(body.Model))
		}
		oldMCP := s.Cfg.MCPConfig
		if strings.TrimSpace(body.MCPConfig) != "" {
			s.Cfg.MCPConfig = strings.TrimSpace(body.MCPConfig)
		}
		if l := strings.TrimSpace(body.Lang); l != "" {
			s.Cfg.Lang = string(i18n.Normalize(l))
		}
		if th := strings.TrimSpace(body.Theme); th == "dark" || th == "light" {
			s.Cfg.Theme = th
		}
		if err := s.Cfg.Save(s.CfgPath); err != nil {
			writeJSON(w, map[string]interface{}{"ok": false, "error": i18n.T(s.lang(), "save_config_failed") + err.Error()})
			return
		}
		resp := map[string]interface{}{"ok": true}
		if s.Cfg.MCPConfig != oldMCP {
			if err := s.MCP.Load(context.Background(), s.Cfg.MCPConfig); err != nil {
				resp["mcp_error"] = err.Error()
			}
			resp["mcp_reloaded"] = true
		}
		writeJSON(w, resp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProviders 返回内置服务商预设（界面下拉的唯一数据源）。
func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "providers": config.Providers()})
}

// lang 返回当前界面语言。
func (s *Server) lang() i18n.Lang { return i18n.Normalize(s.Cfg.Lang) }

func (s *Server) handleServers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"ok":      true,
		"servers": s.MCP.Servers(),
	})
}

func (s *Server) handleServersReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		MCPConfig string `json:"mcp_config"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	path := strings.TrimSpace(body.MCPConfig)
	if path == "" {
		path = s.Cfg.MCPConfig
	} else {
		s.Cfg.MCPConfig = path
		_ = s.Cfg.Save(s.CfgPath)
	}
	err := s.MCP.Load(context.Background(), path)
	resp := map[string]interface{}{"ok": err == nil, "servers": s.MCP.Servers()}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, resp)
}

func (s *Server) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, err := s.pickFolderLocalized()
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if path == "" {
		writeJSON(w, map[string]interface{}{"ok": false, "canceled": true})
		return
	}
	s.Cfg.Workspace = path
	_ = s.Cfg.Save(s.CfgPath)
	writeJSON(w, map[string]interface{}{"ok": true, "path": path})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.APIKey == "" && s.providerNeedsKey() {
		writeJSON(w, map[string]interface{}{"ok": false, "error": i18n.T(s.lang(), "need_api_key_first")})
		return
	}
	c := llm.NewClient(s.Cfg.BaseURL, s.Cfg.APIKey, s.Cfg.Model)
	if p := config.ProviderByID(s.Cfg.Provider); p != nil && len(p.ExtraBody) > 0 {
		c.WithExtra(p.ExtraBody)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	models, err := c.ListModels(ctx)
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "models": models})
}

// providerNeedsKey 判断当前服务商是否必须提供 API Key（本地 Ollama 可留空）。
func (s *Server) providerNeedsKey() bool {
	if p := config.ProviderByID(s.Cfg.Provider); p != nil {
		return p.NeedsKey
	}
	return true
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	emit := func(ev chat.Event) {
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}
	_ = s.Session.Send(r.Context(), body.Message, emit)
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.Session.Reset()
	writeJSON(w, map[string]interface{}{"ok": true})
}

func (s *Server) handleQuit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"ok": true})
	if f := s.quitFn; f != nil {
		go func() {
			time.Sleep(150 * time.Millisecond)
			f()
		}()
	}
}

// SetQuitFunc 注册退出回调（GUI 模式下关闭窗口）。
func (s *Server) SetQuitFunc(f func()) { s.quitFn = f }

// pickFolder 打开 Windows 原生文件夹选择对话框。
// 返回空字符串表示用户取消；出错时返回 error。
func (s *Server) pickFolderLocalized() (string, error) {
	desc := i18n.T(s.lang(), "pick_folder_desc")
	return pickFolderWithDesc(desc)
}

func pickFolder() (string, error) {
	return pickFolderWithDesc(i18n.T(i18n.ZH, "pick_folder_desc"))
}

func pickFolderWithDesc(desc string) (string, error) {
	ps := `Add-Type -AssemblyName System.Windows.Forms;` +
		`$f = New-Object System.Windows.Forms.FolderBrowserDialog;` +
		`$f.Description = '` + desc + `';` +
		`$f.ShowNewFolderButton = $false;` +
		`if ($f.ShowDialog() -eq 'OK') { Write-Output $f.SelectedPath }`
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-STA", "-ExecutionPolicy", "Bypass", "-Command", ps)
	cmd.Dir = filepath.Dir(mustExe())
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%s", i18n.T(i18n.ZH, "pick_folder_timeout"))
		}
		// 用户取消时 PowerShell 正常退出但无输出，这里统一按无输出处理
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line, nil
		}
	}
	return "", nil
}

func mustExe() string {
	p, err := exec.LookPath("powershell")
	if err != nil {
		return "."
	}
	return p
}
