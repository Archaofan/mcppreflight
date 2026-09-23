// MCP点检助手 —— 极简 Windows 桌面工具：
// 在聊天界面中通过 DeepSeek（OpenAI 兼容 API）调用 MCP 工具完成点检。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"mcpcheck/internal/app"
	"mcpcheck/internal/chat"
	"mcpcheck/internal/config"
	"mcpcheck/internal/mcp"
)

var (
	flagHeadless = flag.String("headless", "", "无头模式：直接用配置跑一条指令并打印过程，不打开界面")
	flagServe    = flag.Bool("serve", false, "仅启动本地服务（不打开窗口），用于调试")
	flagPort     = flag.Int("port", 0, "固定端口（默认自动选择）")
	flagConfig   = flag.String("config", "", "配置文件路径（默认 exe 同目录 config.json）")
)

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[mcpcheck] ")

	cfgPath := *flagConfig
	if cfgPath == "" {
		cfgPath = filepath.Join(config.ExeDir(), "config.json")
	}
	setupLogFile(cfgPath)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("读取配置失败: %v", err)
	}
	if cfg.MCPConfig == "" {
		cfg.MCPConfig = filepath.Join(config.ExeDir(), "mcp.json")
	}
	if cfg.Workspace == "" {
		if wd, err := os.Getwd(); err == nil {
			cfg.Workspace = wd
		}
	}

	manager := mcp.NewManager()
	if cfg.MCPConfig != "" {
		if err := manager.Load(context.Background(), cfg.MCPConfig); err != nil {
			log.Printf("加载 mcp.json 失败（%s）: %v", cfg.MCPConfig, err)
		} else {
			log.Printf("已加载 %d 个 MCP 服务器定义，%d 个连接成功", len(manager.Servers()), manager.ConnectedCount())
		}
	}
	session := chat.New(cfg, manager)
	server := app.New(cfg, cfgPath, manager, session)

	switch {
	case *flagHeadless != "":
		runHeadless(session, *flagHeadless)
	case *flagServe:
		if err := server.Start(); err != nil {
			log.Fatalf("启动服务失败: %v", err)
		}
		log.Printf("服务已启动: %s （按 Ctrl+C 退出）", server.URL())
		blockForever()
	default:
		runGUI(server)
	}
}

func runGUI(server *app.Server) {
	if err := server.Start(); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
	url := server.URL()
	log.Printf("本地服务: %s", url)

	// 打开窗口前隐藏控制台窗口（若有）
	hideConsoleWindow()

	w := newWebView(url)
	if w == nil {
		// WebView2 运行时不可用：退回系统浏览器
		log.Printf("WebView2 不可用，改用系统浏览器打开: %s", url)
		openBrowser(url)
		blockForever()
		return
	}
	server.SetQuitFunc(w.Terminate)
	defer w.Destroy()
	w.SetTitle("MCP点检助手")
	w.SetSize(1180, 780, 0)
	w.Navigate(url)
	w.Run()
	log.Printf("窗口已关闭，退出")
}

func runHeadless(session *chat.Session, message string) {
	fmt.Println("=== MCP点检助手 · 无头模式 ===")
	fmt.Printf("指令: %s\n\n", message)
	err := session.Send(context.Background(), message, func(ev chat.Event) {
		switch ev.Type {
		case "assistant_delta":
			fmt.Print(ev.Text)
		case "tool_call":
			fmt.Printf("\n>>> 调用工具 [%s] 参数: %s\n", ev.Name, ev.Args)
		case "tool_result":
			fmt.Printf("<<< 工具返回 [%s]:\n%s\n", ev.Name, truncateForPrint(ev.Text))
		case "error":
			fmt.Printf("\n[错误] %s\n", ev.Text)
		case "done":
			fmt.Println("\n=== 完成 ===")
		}
	})
	if err != nil {
		fmt.Printf("\n[失败] %v\n", err)
		os.Exit(1)
	}
}

func truncateForPrint(s string) string {
	r := []rune(s)
	if len(r) > 3000 {
		return string(r[:3000]) + fmt.Sprintf("\n…（控制台展示截断，完整内容共 %d 字；图形界面中可完整查看）", len(r))
	}
	return s
}

func setupLogFile(cfgPath string) {
	// 日志优先写 exe 目录；不可写时退回临时目录
	dir := filepath.Dir(cfgPath)
	logPath := filepath.Join(dir, "mcpcheck.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		tmp := filepath.Join(os.TempDir(), "mcpcheck.log")
		f, err = os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		log.Printf("日志文件: %s", tmp)
	}
	log.SetOutput(f)
	log.Printf("===== 启动 ===== 配置: %s", strings.TrimSpace(cfgPath))
}
