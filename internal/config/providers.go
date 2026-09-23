package config

import "strings"

// Provider 描述一个 OpenAI 兼容的模型服务商预设。
// 新增厂商只需在此表中加一行（含 base_url / 模型清单 / 工具调用支持度），
// UI 下拉、模型候选、提示语都由 /api/providers 统一下发，无需改动其他代码。
type Provider struct {
	ID      string `json:"id"`
	Name    string `json:"name"`    // 中文名
	NameEN  string `json:"name_en"` // 英文名
	BaseURL string `json:"base_url"`
	Models  []string `json:"models"`

	// Tools 表示工具调用（function calling）支持度：full / partial / none
	Tools       string `json:"tools"`
	ToolsNote   string `json:"tools_note,omitempty"`
	ToolsNoteEN string `json:"tools_note_en,omitempty"`
	KeyHint     string `json:"key_hint,omitempty"`
	KeyHintEN   string `json:"key_hint_en,omitempty"`

	// ModelsAPI 表示 GET {base_url}/models 是否可用（决定「获取」按钮的预期）
	ModelsAPI bool `json:"models_api"`

	// NeedsKey 为 false 时允许 Key 留空（如本地 Ollama）
	NeedsKey bool `json:"needs_key"`
	// SendToolChoice 为 false 时不下发 tool_choice（如 Ollama 不支持该参数）
	SendToolChoice bool `json:"send_tool_choice"`

	// ExtraBody 是厂商私有的附加请求参数（如星火 tool_calls_switch、硅基流动 enable_thinking）
	ExtraBody map[string]interface{} `json:"-"`

	DocsURL string `json:"docs_url,omitempty"`
}

// customProvider 是「自定义」占位（用户手工填写 base_url，兼容 one-api/new-api 等中转）。
var customProvider = Provider{
	ID:              "custom",
	Name:            "自定义",
	NameEN:          "Custom",
	BaseURL:         "",
	Tools:           "full",
	ModelsAPI:       true,
	NeedsKey:        true,
	SendToolChoice:  true,
	ToolsNote:       "任意 OpenAI 兼容接口（含 one-api / new-api 等中转），工具调用能力取决于后端模型",
	ToolsNoteEN:     "Any OpenAI-compatible endpoint (including one-api / new-api relays); tool calling depends on the backing model",
	KeyHint:         "Key 由你的服务商或中转提供",
	KeyHintEN:       "Use the key issued by your provider or relay",
}

// providerList 是内置服务商预设（顺序即界面下拉顺序，DeepSeek 为默认）。
// base_url 均按官方文档逐字符核对，未做任何字符串拼接假设。
var providerList = []Provider{
	{
		ID: "deepseek", Name: "DeepSeek", NameEN: "DeepSeek",
		BaseURL: "https://api.deepseek.com",
		Models:  []string{"deepseek-flash", "deepseek-v4-pro"},
		Tools:   "full", ModelsAPI: true, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "注意：deepseek-chat / deepseek-reasoner 已停用，旧配置会自动迁移到 deepseek-flash",
		ToolsNoteEN: "Note: deepseek-chat / deepseek-reasoner are discontinued; legacy configs are migrated to deepseek-flash automatically",
		KeyHint:     "Key 在 platform.deepseek.com 创建",
		KeyHintEN:   "Create your key at platform.deepseek.com",
		DocsURL:     "https://api-docs.deepseek.com/",
	},
	{
		ID: "qwen", Name: "阿里云百炼（通义千问）", NameEN: "Aliyun DashScope (Qwen)",
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Models:  []string{"qwen3.8-max", "qwen3.8-flash", "qwen3.7-plus", "qwen3.7-flash", "qwen-plus", "qwen-max", "qwen-flash", "qwen-turbo", "qwen-long"},
		Tools:   "full", ModelsAPI: false, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "通用商业模型均支持 Function Calling；qwen-long 不支持",
		ToolsNoteEN: "All general commercial models support function calling (qwen-long does not)",
		KeyHint:     "Key 在 bailian.console.aliyun.com 创建（与地域绑定，跨地域会 401）",
		KeyHintEN:   "Create your key at bailian.console.aliyun.com (region-bound)",
		DocsURL:     "https://help.aliyun.com/zh/model-studio/qwen-api-via-openai-chat-completions",
	},
	{
		ID: "glm", Name: "智谱 BigModel（GLM）", NameEN: "Zhipu BigModel (GLM)",
		BaseURL: "https://open.bigmodel.cn/api/paas/v4",
		Models:  []string{"glm-5.3", "glm-5.3-flash", "glm-5.3-flashx", "glm-5.2", "glm-5.1", "glm-5", "glm-5-turbo", "glm-4.7", "glm-4.7-flash", "glm-4.5-air", "glm-z1-air"},
		Tools:   "full", ModelsAPI: false, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "glm-5.x 均支持 Function Calling；tool_choice 仅支持 auto（本工具即下发 auto）",
		ToolsNoteEN: "glm-5.x models support function calling; tool_choice only accepts auto (which this app sends)",
		KeyHint:     "Key 在 open.bigmodel.cn 用户中心创建",
		KeyHintEN:   "Create your key at open.bigmodel.cn",
		DocsURL:     "https://docs.bigmodel.cn/cn/guide/develop/openai/introduction",
	},
	{
		ID: "kimi", Name: "Moonshot（Kimi）", NameEN: "Moonshot (Kimi)",
		BaseURL: "https://api.moonshot.cn/v1",
		Models:  []string{"kimi-k3", "kimi-k2.7-code", "kimi-k2.7-code-highspeed", "kimi-k2.6"},
		Tools:   "full", ModelsAPI: true, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "kimi-k3 支持 auto/none/required；k2.x 仅支持 auto/none",
		ToolsNoteEN: "kimi-k3 supports auto/none/required; k2.x supports auto/none only",
		KeyHint:     "Key 在 platform.kimi.com 创建（与 platform.kimi.ai 的 Key 不通用）",
		KeyHintEN:   "Create your key at platform.kimi.com (not interchangeable with platform.kimi.ai)",
		DocsURL:     "https://platform.kimi.com/docs/api/overview",
	},
	{
		ID: "spark", Name: "讯飞星火（Spark）", NameEN: "iFlytek Spark",
		BaseURL: "https://spark-api-open.xf-yun.com/v1",
		Models:  []string{"4.0Ultra", "max-32k", "pro-128k", "generalv3.5", "generalv3", "lite"},
		Tools:   "partial", ModelsAPI: false, NeedsKey: true, SendToolChoice: true,
		ExtraBody: map[string]interface{}{"tool_calls_switch": true},
		ToolsNote:   "仅 Max/Ultra 支持 Function Call，且需开启 tool_calls_switch（本工具已自动带上）；Key 为控制台按模型版本下发的 APIPassword",
		ToolsNoteEN: "Function calling works on Max/Ultra only and needs tool_calls_switch (sent automatically); the key is the per-model APIPassword from the console",
		KeyHint:     "在 console.xfyun.cn 按模型版本获取 APIPassword（不是 APPID/APISecret）",
		KeyHintEN:   "Get the per-model APIPassword from console.xfyun.cn (not APPID/APISecret)",
		DocsURL:     "https://www.xfyun.cn/doc/spark/HTTP%E8%B0%83%E7%94%A8%E6%96%87%E6%A1%A3.html",
	},
	{
		ID: "minimax", Name: "MiniMax", NameEN: "MiniMax",
		BaseURL: "https://api.minimax.cn/v1",
		Models:  []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M2.7-highspeed", "MiniMax-M2.5", "MiniMax-M2.5-highspeed", "MiniMax-M2.1", "MiniMax-M2.1-highspeed", "MiniMax-M2"},
		Tools:   "full", ModelsAPI: true, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "支持 tools；M2.x 思考模式无法关闭，思考内容可能混在回复中",
		ToolsNoteEN: "tools supported; thinking cannot be disabled on M2.x and may be mixed into the reply",
		KeyHint:     "Key 在 platform.minimax.cn 用户中心创建（注意域名已迁到 api.minimax.cn）",
		KeyHintEN:   "Create your key at platform.minimax.cn (the API domain is now api.minimax.cn)",
		DocsURL:     "https://platform.minimax.cn/docs/api-reference/text-openai-api",
	},
	{
		ID: "doubao", Name: "火山方舟（豆包）", NameEN: "Volcano Ark (Doubao)",
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		Models: []string{
			"doubao-seed-2-1-pro-260628", "doubao-seed-2-0-pro-260215", "doubao-seed-2-0-lite-260215",
			"doubao-seed-2-0-mini-260215", "doubao-seed-1.6-250615", "doubao-1.5-pro-32k", "doubao-1.5-lite-32k",
		},
		Tools: "partial", ModelsAPI: false, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "文本模型（pro/lite/seed 系列）支持 Function Calling，vision 模型不支持；model 也可填控制台的推理接入点 ID（ep- 开头），需先在控制台开通模型",
		ToolsNoteEN: "Text models (pro/lite/seed) support function calling; vision models do not. The model field also accepts an inference endpoint ID (ep-...) — the model must be enabled in the console first",
		KeyHint:     "Key 在火山方舟控制台「API Key 管理」创建",
		KeyHintEN:   "Create your key in the Ark console under API Key Management",
		DocsURL:     "https://www.volcengine.com/docs/82379/1330310",
	},
	{
		ID: "qianfan", Name: "百度千帆（文心）", NameEN: "Baidu Qianfan (ERNIE)",
		BaseURL: "https://qianfan.baidubce.com/v2",
		Models: []string{
			"ernie-4.5-turbo-128k", "ernie-4.5-turbo-32k", "ernie-4.5-8k", "ernie-4.0-turbo-128k",
			"ernie-4.0-turbo-8k", "ernie-x1-turbo-32k", "ernie-speed-128k", "ernie-lite-8k",
		},
		Tools: "partial", ModelsAPI: false, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "turbo/x1 系列支持 Function Calling；speed/lite/tiny 不支持。请使用 v2 接口与静态 API Key（bce-v3/ALTAK- 开头）",
		ToolsNoteEN: "turbo/x1 series support function calling; speed/lite/tiny do not. Use the v2 endpoint with a static API key (bce-v3/ALTAK-...)",
		KeyHint:     "Key 在百度千帆「系统管理」创建（永久有效）",
		KeyHintEN:   "Create your key under System Management in the Qianfan console",
		DocsURL:     "https://cloud.baidu.com/doc/qianfan/s/Hmh4suq26",
	},
	{
		ID: "hunyuan", Name: "腾讯混元", NameEN: "Tencent Hunyuan",
		BaseURL: "https://api.hunyuan.cloud.tencent.com/v1",
		Models: []string{
			"hunyuan-a13b", "hunyuan-turbos-latest", "hunyuan-t1", "hunyuan-functioncall",
			"hunyuan-standard", "hunyuan-lite", "hunyuan-vision-1.5-instruct",
		},
		Tools: "partial", ModelsAPI: false, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "仅 turbos/t1/functioncall 系列支持工具调用，lite 不支持",
		ToolsNoteEN: "Only turbos/t1/functioncall series support tool calling; lite does not",
		KeyHint:     "Key 在腾讯云混元控制台创建（平台正在迁移至 TokenHub）",
		KeyHintEN:   "Create your key in the Tencent Hunyuan console (the platform is migrating to TokenHub)",
		DocsURL:     "https://cloud.tencent.com/document/product/1729/111007",
	},
	{
		ID: "siliconflow", Name: "硅基流动（SiliconFlow）", NameEN: "SiliconFlow",
		BaseURL: "https://api.siliconflow.com/v1",
		Models: []string{
			"deepseek-ai/DeepSeek-V3.2", "deepseek-ai/DeepSeek-V3.1", "deepseek-ai/DeepSeek-R1",
			"Qwen/Qwen3-32B", "Qwen/Qwen3-235B-A22B", "Qwen/Qwen3-Coder-480B-A35B-Instruct",
			"zai-org/GLM-5.1", "moonshotai/Kimi-K2.6", "MiniMaxAI/MiniMax-M2.5",
			"tencent/Hunyuan-A13B-Instruct", "baidu/ERNIE-4.5-300B-A47B", "openai/gpt-oss-120b",
		},
		Tools:      "partial",
		ModelsAPI:  true,
		NeedsKey:   true,
		SendToolChoice: true,
		// enable_thinking 默认开启，部分模型（DeepSeek-V3.1 等）关闭思考才能用 function calling
		ExtraBody: map[string]interface{}{"enable_thinking": false},
		ToolsNote:   "工具调用能力按模型分化；已自动下发 enable_thinking=false 以保证 Function Calling 可用",
		ToolsNoteEN: "Tool support varies by model; enable_thinking=false is sent automatically so function calling works",
		KeyHint:     "Key 在 cloud.siliconflow.cn 创建（新端点为 api.siliconflow.com）",
		KeyHintEN:   "Create your key at cloud.siliconflow.cn (current endpoint: api.siliconflow.com)",
		DocsURL:     "https://docs.siliconflow.com/cn/userguide/capabilities/text-generation",
	},
	{
		ID: "ollama", Name: "Ollama（本地）", NameEN: "Ollama (local)",
		BaseURL: "http://localhost:11434/v1",
		Models:  []string{},
		Tools:   "full", ModelsAPI: true, NeedsKey: false, SendToolChoice: false,
		ToolsNote:   "模型由本地 ollama pull 决定；不支持 tool_choice（本工具已自动忽略）",
		ToolsNoteEN: "Models come from your local ollama pull; tool_choice is unsupported and skipped automatically",
		KeyHint:     "本地服务无需 Key，可留空（或随意填写）",
		KeyHintEN:   "No key needed for the local service; leave it empty",
		DocsURL:     "https://docs.ollama.com/api/openai-compatibility",
	},
	{
		ID: "openrouter", Name: "OpenRouter", NameEN: "OpenRouter",
		BaseURL: "https://openrouter.ai/api/v1",
		Models:  []string{},
		Tools:   "full", ModelsAPI: true, NeedsKey: true, SendToolChoice: true,
		ToolsNote:   "模型为 厂商/模型 格式；工具支持按模型而定，可用「获取」拉取列表",
		ToolsNoteEN: "Models use the vendor/model format; tool support is per-model — use Fetch to list them",
		KeyHint:     "Key 在 openrouter.ai/keys 创建",
		KeyHintEN:   "Create your key at openrouter.ai/keys",
		DocsURL:     "https://openrouter.ai/docs/quickstart",
	},
}

// Providers 返回全部服务商预设（含末尾的「自定义」）。
func Providers() []Provider {
	out := make([]Provider, 0, len(providerList)+1)
	out = append(out, providerList...)
	out = append(out, customProvider)
	return out
}

// ProviderByID 按 ID 查找预设；找不到返回 nil。
func ProviderByID(id string) *Provider {
	for i := range providerList {
		if providerList[i].ID == id {
			p := providerList[i]
			return &p
		}
	}
	if id == customProvider.ID {
		p := customProvider
		return &p
	}
	return nil
}

// InferProvider 根据 base_url 推断服务商（用于兼容旧版没有 provider 字段的配置）。
func InferProvider(baseURL string) string {
	b := strings.ToLower(strings.TrimSpace(baseURL))
	switch {
	case strings.Contains(b, "deepseek.com"):
		return "deepseek"
	case strings.Contains(b, "dashscope.aliyuncs.com"):
		return "qwen"
	case strings.Contains(b, "bigmodel.cn"):
		return "glm"
	case strings.Contains(b, "moonshot."):
		return "kimi"
	case strings.Contains(b, "volces.com"):
		return "doubao"
	case strings.Contains(b, "baidubce.com"):
		return "qianfan"
	case strings.Contains(b, "hunyuan.cloud.tencent.com"):
		return "hunyuan"
	case strings.Contains(b, "xf-yun.com"):
		return "spark"
	case strings.Contains(b, "minimax."):
		return "minimax"
	case strings.Contains(b, "siliconflow."):
		return "siliconflow"
	case strings.Contains(b, "localhost:11434") || strings.Contains(b, "127.0.0.1:11434"):
		return "ollama"
	case strings.Contains(b, "openrouter.ai"):
		return "openrouter"
	default:
		return customProvider.ID
	}
}

// deprecatedModels 记录已停用的模型 ID 到现行 ID 的映射（保证旧配置继续可用）。
var deprecatedModels = map[string]string{
	"deepseek-chat":     "deepseek-flash",
	"deepseek-reasoner": "deepseek-flash",
}

// MigrateModel 把已停用的模型 ID 迁移到现行 ID；无需迁移时原样返回。
func MigrateModel(model string) string {
	if m, ok := deprecatedModels[strings.TrimSpace(model)]; ok {
		return m
	}
	return model
}
