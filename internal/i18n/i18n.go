// Package i18n 提供界面/提示文案的简体中文与英文对照。
// 语言随配置（config.json 的 lang 字段）切换，默认简体中文，保证既有行为不变。
package i18n

import "strings"

// Lang 是界面语言标识。
type Lang string

const (
	// ZH 简体中文（默认）
	ZH Lang = "zh-CN"
	// EN English
	EN Lang = "en"
)

// Normalize 把任意输入归一化为受支持的语言；无法识别时返回简体中文。
func Normalize(s string) Lang {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "en", "en-us", "english":
		return EN
	default:
		return ZH
	}
}

// messages 是全部文案的对照表。key 在各语言中必须齐全（由测试保证）。
var messages = map[string]map[Lang]string{
	/* ---- 会话/模型相关 ---- */
	"err_no_api_key": {
		ZH: "尚未配置 API Key，请先在左侧「模型设置」中填写",
		EN: "No API key configured yet. Please fill it in Model Settings on the left.",
	},
	"err_no_mcp": {
		ZH: "当前没有已连接的 MCP 服务器，请检查 mcp.json 配置后点击「重新加载」",
		EN: "No MCP server is connected. Check mcp.json and click Reload.",
	},
	"err_too_many_rounds": {
		ZH: "工具调用轮次过多，已停止。请尝试把需求描述得更具体。",
		EN: "Too many tool-calling rounds; stopped. Please describe your request more specifically.",
	},
	"tool_failed_prefix": {
		ZH: "工具调用失败：",
		EN: "Tool call failed: ",
	},
	"err_no_mcp_tools": {
		ZH: "当前没有可用的 MCP 工具，请检查 mcp.json 配置后点击「重新加载」",
		EN: "No MCP tools available. Check mcp.json and click Reload.",
	},

	/* ---- 配置/接口相关 ---- */
	"need_api_key_first": {
		ZH: "请先填写并保存 API Key",
		EN: "Please enter and save your API key first.",
	},
	"save_config_failed": {
		ZH: "保存配置失败: ",
		EN: "Failed to save config: ",
	},
	"invalid_json_body": {
		ZH: "请求体不是合法 JSON",
		EN: "Request body is not valid JSON.",
	},

	/* ---- 系统提示（决定报告语言，随界面语言切换） ---- */
	"sys_intro": {
		ZH: "你是「MCP点检助手」，运行在用户本机。你的任务是通过 MCP 工具帮助用户对软件工程目录执行点检，并输出中文点检报告（Markdown 格式）。\n",
		EN: "You are MCP Preflight, running locally on the user's machine. Your task is to help the user inspect software engineering directories through MCP tools, and to output the inspection report in English (Markdown).\n",
	},
	"sys_workspace": {
		ZH: "当前工作目录（工作区）为：%s 。当工具需要软件工程目录/路径参数时，默认使用该工作区路径，除非用户明确指定了其他路径。\n",
		EN: "The current working directory (workspace) is: %s . When a tool needs a software project directory/path argument, use this workspace path by default unless the user explicitly specifies another path.\n",
	},
	"sys_rules": {
		ZH: "规则：1) 需要点检时调用相应 MCP 工具，不要臆造点检结果；2) 工具返回后，基于真实返回内容整理成结构化报告（问题清单、风险等级、修复建议）；3) 工具调用失败时如实说明错误信息。",
		EN: "Rules: 1) Call the appropriate MCP tools when inspection is needed; never fabricate results. 2) After a tool returns, organize the real output into a structured report (issue list, risk levels, fix suggestions). 3) If a tool call fails, report the error honestly.",
	},

	/* ---- 无头模式 ---- */
	"headless_title": {
		ZH: "=== MCP点检助手 · 无头模式 ===",
		EN: "=== MCP Preflight · headless mode ===",
	},
	"headless_command": {
		ZH: "指令: %s",
		EN: "Command: %s",
	},
	"headless_tool_call": {
		ZH: "\n>>> 调用工具 [%s] 参数: %s\n",
		EN: "\n>>> Calling tool [%s] args: %s\n",
	},
	"headless_tool_result": {
		ZH: "\n<<< 工具返回 [%s]:\n%s\n",
		EN: "\n<<< Tool result [%s]:\n%s\n",
	},
	"headless_error": {
		ZH: "\n[错误] %s\n",
		EN: "\n[Error] %s\n",
	},
	"headless_done": {
		ZH: "\n=== 完成 ===",
		EN: "\n=== Done ===",
	},
	"headless_failed": {
		ZH: "\n[失败] %v\n",
		EN: "\n[Failed] %v\n",
	},
	"headless_console_truncated": {
		ZH: "\n…（控制台展示截断，完整内容共 %d 字；图形界面中可完整查看）",
		EN: "\n…(console output truncated, %d characters in total; the GUI shows it in full)",
	},

	/* ---- 文件夹选择对话框 ---- */
	"pick_folder_desc": {
		ZH: "请选择工作区文件夹（点检目标目录）",
		EN: "Select the workspace folder (inspection target directory)",
	},
	"pick_folder_timeout": {
		ZH: "选择文件夹超时",
		EN: "Folder selection timed out.",
	},
}

// T 返回指定语言下的文案；key 不存在时返回 key 本身（便于发现遗漏）。
func T(l Lang, key string) string {
	if m, ok := messages[key]; ok {
		if s, ok := m[l]; ok {
			return s
		}
	}
	return key
}

// Keys 返回全部文案 key（供测试校验中英文完整性）。
func Keys() []string {
	out := make([]string, 0, len(messages))
	for k := range messages {
		out = append(out, k)
	}
	return out
}
