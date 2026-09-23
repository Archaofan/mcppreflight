//go:build windows

package main

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	webview "github.com/jchv/go-webview2"

	"golang.org/x/sys/windows"
)

// newWebView 创建 WebView2 窗口；运行时不可用时返回 nil（调用方走浏览器兜底）。
// 注意：webview2 库在 settings 出错时会 log.Fatal，这里用 recover 兜底。
func newWebView(url string) (w webview.WebView) {
	defer func() {
		if r := recover(); r != nil {
			w = nil
		}
	}()
	dataPath := ""
	if exe, err := os.Executable(); err == nil {
		dataPath = exe + ".webview2"
	}
	opts := webview.WebViewOptions{
		Debug: false,
		WindowOptions: webview.WindowOptions{
			Title:  "MCP点检助手",
			Width:  1280,
			Height: 820,
			Center: true,
			// 1 = rsrc 嵌入的 RT_GROUP_ICON 资源 ID（见 tools/icon 与 build.ps1）
			IconId: 1,
		},
	}
	if dataPath != "" {
		opts.DataPath = dataPath
	}
	return webview.NewWithOptions(opts)
}

// openBrowser 用系统默认浏览器打开 URL。
func openBrowser(url string) {
	cmd := exec.Command("cmd", "/c", "start", "", url)
	_ = cmd.Start()
}

// blockForever 阻塞当前 goroutine（浏览器兜底模式下保持进程存活）。
func blockForever() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
}

// hideConsoleWindow 隐藏控制台窗口，让 GUI 模式像普通桌面程序一样运行。
func hideConsoleWindow() {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	user32 := windows.NewLazySystemDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd == 0 {
		return
	}
	const swHide = 0
	showWindow.Call(hwnd, swHide)
}
