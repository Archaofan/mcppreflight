package config

import (
	"strings"
	"testing"
)

func TestProvidersIntegrity(t *testing.T) {
	ps := Providers()
	if len(ps) < 10 {
		t.Fatalf("内置服务商数量偏少: %d", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if p.ID == "" {
			t.Fatalf("存在空 ID 的服务商: %+v", p)
		}
		if seen[p.ID] {
			t.Fatalf("服务商 ID 重复: %s", p.ID)
		}
		seen[p.ID] = true
		if p.Name == "" || p.NameEN == "" {
			t.Fatalf("%s 缺少中/英文名", p.ID)
		}
		if p.ID != "custom" {
			if !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
				t.Fatalf("%s base_url 非法: %s", p.ID, p.BaseURL)
			}
			if strings.HasSuffix(p.BaseURL, "/") {
				t.Fatalf("%s base_url 不应以 / 结尾: %s", p.ID, p.BaseURL)
			}
		}
		switch p.Tools {
		case "full", "partial", "none":
		default:
			t.Fatalf("%s tools 字段取值非法: %s", p.ID, p.Tools)
		}
		if len(p.Models) == 0 && p.ID != "custom" && p.ID != "ollama" && p.ID != "openrouter" {
			t.Fatalf("%s 缺少内置模型清单", p.ID)
		}
	}
	// DeepSeek 必须是第一个（默认厂商）
	if ps[0].ID != "deepseek" {
		t.Fatalf("第一个服务商应为 deepseek，实际为 %s", ps[0].ID)
	}
	// 末尾必须是 custom
	if ps[len(ps)-1].ID != "custom" {
		t.Fatalf("最后一个服务商应为 custom，实际为 %s", ps[len(ps)-1].ID)
	}
}

// 关键厂商的 base_url 按官方文档逐字符核对（防止回归）
func TestProviderBaseURLs(t *testing.T) {
	cases := map[string]string{
		"deepseek":    "https://api.deepseek.com",               // 无 /v1
		"qwen":        "https://dashscope.aliyuncs.com/compatible-mode/v1",
		"glm":         "https://open.bigmodel.cn/api/paas/v4",
		"kimi":        "https://api.moonshot.cn/v1",
		"spark":       "https://spark-api-open.xf-yun.com/v1",
		"minimax":     "https://api.minimax.cn/v1",
		"siliconflow": "https://api.siliconflow.com/v1",
		"ollama":      "http://localhost:11434/v1",
		"openrouter":  "https://openrouter.ai/api/v1", // 是 /api/v1
		"doubao":      "https://ark.cn-beijing.volces.com/api/v3",
		"qianfan":     "https://qianfan.baidubce.com/v2",
		"hunyuan":     "https://api.hunyuan.cloud.tencent.com/v1",
	}
	for id, want := range cases {
		p := ProviderByID(id)
		if p == nil {
			t.Fatalf("缺少服务商 %s", id)
		}
		if p.BaseURL != want {
			t.Fatalf("%s base_url 应为 %s，实际 %s", id, want, p.BaseURL)
		}
	}
	// DeepSeek 默认模型必须是现行 ID
	ds := ProviderByID("deepseek")
	if len(ds.Models) == 0 || ds.Models[0] != "deepseek-flash" {
		t.Fatalf("DeepSeek 默认模型应为 deepseek-flash，实际 %v", ds.Models)
	}
	for _, m := range ds.Models {
		if m == "deepseek-chat" || m == "deepseek-reasoner" {
			t.Fatalf("DeepSeek 模型清单不应包含已停用 ID: %s", m)
		}
	}
}

// 能力降级与私有参数（调研结论落地）
func TestProviderCapabilities(t *testing.T) {
	// 星火：必须带 tool_calls_switch，否则工具调用不会走标准 tool_calls 字段
	spark := ProviderByID("spark")
	if spark.ExtraBody["tool_calls_switch"] != true {
		t.Fatalf("星火应自动下发 tool_calls_switch=true")
	}
	if spark.Tools != "partial" {
		t.Fatalf("星火工具调用支持度应为 partial")
	}
	// 硅基流动：enable_thinking 默认开启，需显式关闭以保证 Function Calling
	sf := ProviderByID("siliconflow")
	if sf.ExtraBody["enable_thinking"] != false {
		t.Fatalf("硅基流动应自动下发 enable_thinking=false")
	}
	// Ollama：不需要 Key、不支持 tool_choice
	ollama := ProviderByID("ollama")
	if ollama.NeedsKey {
		t.Fatalf("Ollama 应允许空 Key")
	}
	if ollama.SendToolChoice {
		t.Fatalf("Ollama 不应下发 tool_choice")
	}
	// 其余内置厂商都必须要 Key 且默认下发 tool_choice
	for _, p := range providerList {
		if p.ID == "ollama" {
			continue
		}
		if !p.NeedsKey || !p.SendToolChoice {
			t.Fatalf("%s 的 Key/tool_choice 能力标注异常", p.ID)
		}
	}
}

func TestInferProvider(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com":                              "deepseek",
		"https://dashscope.aliyuncs.com/compatible-mode/v1":     "qwen",
		"https://open.bigmodel.cn/api/paas/v4":                  "glm",
		"https://api.moonshot.cn/v1":                            "kimi",
		"https://ark.cn-beijing.volces.com/api/v3":              "doubao",
		"https://qianfan.baidubce.com/v2":                       "qianfan",
		"https://api.hunyuan.cloud.tencent.com/v1":              "hunyuan",
		"http://localhost:11434/v1":                             "ollama",
		"https://openrouter.ai/api/v1":                          "openrouter",
		"https://my-relay.example.com/v1":                       "custom",
	}
	for url, want := range cases {
		if got := InferProvider(url); got != want {
			t.Fatalf("InferProvider(%s) = %s，期望 %s", url, got, want)
		}
	}
}

func TestMigrateModel(t *testing.T) {
	if got := MigrateModel("deepseek-chat"); got != "deepseek-flash" {
		t.Fatalf("deepseek-chat 应迁移到 deepseek-flash，实际 %s", got)
	}
	if got := MigrateModel("deepseek-reasoner"); got != "deepseek-flash" {
		t.Fatalf("deepseek-reasoner 应迁移到 deepseek-flash，实际 %s", got)
	}
	if got := MigrateModel("deepseek-flash"); got != "deepseek-flash" {
		t.Fatalf("deepseek-flash 不应被改动，实际 %s", got)
	}
	if got := MigrateModel("glm-5.3"); got != "glm-5.3" {
		t.Fatalf("其他模型不应被改动，实际 %s", got)
	}
}

// 旧版配置（没有 provider/lang/theme 字段）应能无缝加载并补齐默认值
func TestLoadLegacyConfig(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	legacy := `{"base_url":"https://api.deepseek.com","api_key":"sk-old","model":"deepseek-chat","workspace":"D:\\w"}`
	if err := writeFile(path, legacy); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Provider != "deepseek" {
		t.Fatalf("provider 应推断为 deepseek，实际 %s", c.Provider)
	}
	if c.Model != "deepseek-flash" {
		t.Fatalf("旧模型 ID 应自动迁移，实际 %s", c.Model)
	}
	if c.Lang != "zh-CN" || c.Theme != "light" {
		t.Fatalf("lang/theme 应补齐默认值，实际 %s/%s", c.Lang, c.Theme)
	}
	if c.APIKey != "sk-old" {
		t.Fatalf("旧配置的 API Key 丢失")
	}
}
