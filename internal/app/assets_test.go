package app

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// HTML 中通过 data-i18n / data-i18n-title / data-i18n-ph 引用的 key
// 必须在前端 I18N 字典中存在，否则切换语言后会漏刷或显示 key 原文。
func TestHTMLI18NKeysExistInFrontendDict(t *testing.T) {
	html, err := fs.ReadFile(assetsFS, "assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	js, err := fs.ReadFile(assetsFS, "assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`data-i18n(?:-title|-ph)?="([^"]+)"`)
	found := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(html), -1) {
		found[m[1]] = true
	}
	if len(found) < 15 {
		t.Fatalf("HTML 中的 i18n 标注过少: %d", len(found))
	}
	// app.js 的 I18N 字典里定义的 key（形如  key: "..." ）
	dictRe := regexp.MustCompile(`(?m)^\s{6}([a-z0-9_]+):\s*"`)
	dict := map[string]bool{}
	for _, m := range dictRe.FindAllStringSubmatch(string(js), -1) {
		dict[m[1]] = true
	}
	if len(dict) < 40 {
		t.Fatalf("前端字典 key 过少: %d", len(dict))
	}
	var missing []string
	for k := range found {
		if !dict[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("HTML 引用了但前端字典缺失的 key: %v", missing)
	}
}

// HTML 中的元素 id 必须与 app.js 里 $(...) 引用的一致（防止改名后静默失效）
func TestHTMLIDsUsedByJS(t *testing.T) {
	html, err := fs.ReadFile(assetsFS, "assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	js, err := fs.ReadFile(assetsFS, "assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	need := []string{
		"cfg-provider", "cfg-base", "cfg-key", "cfg-model", "model-list", "provider-note",
		"btn-models", "btn-save-cfg", "cfg-msg", "btn-pick", "ws-path", "cfg-mcp",
		"btn-reload", "cfg-lang", "cfg-theme", "btn-reset", "chat", "input", "btn-send",
		"panel-resizer", "mcp-panel", "mcp-log", "mcp-empty", "btn-clear-tools", "server-list",
	}
	htmlS := string(html)
	for _, id := range need {
		if !strings.Contains(htmlS, `id="`+id+`"`) {
			t.Fatalf("HTML 缺少元素 #%s", id)
		}
		if !strings.Contains(string(js), `"`+id+`"`) {
			t.Fatalf("app.js 未引用 #%s", id)
		}
	}
}

// 首页不应引用任何外部资源（内网离线可用）
func TestNoExternalResources(t *testing.T) {
	html, err := fs.ReadFile(assetsFS, "assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	for _, bad := range []string{`src="http`, `href="http`, "@import", "url(http", "cdn.", "unpkg", "jsdelivr"} {
		if strings.Contains(s, bad) {
			t.Fatalf("首页不应包含外部引用: %s", bad)
		}
	}
}

// 主题只应通过 CSS 变量实现：body.dark 覆盖变量即可
func TestThemeUsesOnlyCSSVariables(t *testing.T) {
	css, err := fs.ReadFile(assetsFS, "assets/style.css")
	if err != nil {
		t.Fatal(err)
	}
	s := string(css)
	start := strings.Index(s, "body.dark {")
	if start < 0 {
		t.Fatal("样式表应定义 body.dark 覆盖")
	}
	// 取出 body.dark { ... } 的完整块（按花括号配平）
	depth, end := 0, -1
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end > 0 {
			break
		}
	}
	if end < 0 {
		t.Fatal("body.dark 块花括号不配平")
	}
	block := s[start:end]
	// 暗色覆盖块里只允许自定义属性（--xxx: ...）声明，不应出现普通属性与选择器
	varRe := regexp.MustCompile(`^\s*--[\w-]+\s*:`)
	for _, line := range strings.Split(block, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || l == "body.dark {" || l == "}" {
			continue
		}
		if !varRe.MatchString(l) {
			t.Fatalf("暗色主题块里出现了非变量声明: %q", l)
		}
	}
	if !strings.Contains(block, "--") {
		t.Fatal("暗色块应覆盖 CSS 变量")
	}
}
