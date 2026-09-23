package i18n

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]Lang{
		"":        ZH,
		"zh-CN":   ZH,
		"zh":      ZH,
		"ZH-CN":   ZH,
		"en":      EN,
		"en-US":   EN,
		"English": EN,
		"fr":      ZH, // 不支持的语言回退中文
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Fatalf("Normalize(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// 所有文案 key 在中英文下都必须存在且非空，避免切换语言后出现 key 原文
func TestMessagesComplete(t *testing.T) {
	for _, k := range Keys() {
		m := messages[k]
		if m == nil {
			t.Fatalf("key %s 没有文案表", k)
		}
		for _, lang := range []Lang{ZH, EN} {
			s, ok := m[lang]
			if !ok {
				t.Fatalf("key %s 缺少 %s 文案", k, lang)
			}
			if s == "" {
				t.Fatalf("key %s 的 %s 文案为空", k, lang)
			}
		}
	}
}

func TestT(t *testing.T) {
	if got := T(ZH, "err_no_api_key"); got == "err_no_api_key" || got == "" {
		t.Fatalf("中文文案异常: %q", got)
	}
	if got := T(EN, "err_no_api_key"); got == "err_no_api_key" || got == "" {
		t.Fatalf("英文文案异常: %q", got)
	}
	if T(ZH, "err_no_api_key") == T(EN, "err_no_api_key") {
		t.Fatal("中英文文案不应相同")
	}
	// 未知 key 原样返回（便于发现遗漏）
	if got := T(ZH, "no_such_key"); got != "no_such_key" {
		t.Fatalf("未知 key 应原样返回，实际 %q", got)
	}
}

// 带占位符的文案（如系统提示中的工作区路径）
func TestTemplateMessages(t *testing.T) {
	zh := T(ZH, "sys_workspace")
	if zh == "" {
		t.Fatal("sys_workspace 中文文案为空")
	}
	if !contains(zh, "%s") {
		t.Fatalf("sys_workspace 应包含 %%s 占位符: %q", zh)
	}
	en := T(EN, "sys_workspace")
	if !contains(en, "%s") {
		t.Fatalf("英文 sys_workspace 应包含 %%s 占位符: %q", en)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
