// Package chat 编排「用户消息 → LLM → MCP 工具调用 → 报告」的完整流程。
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"mcpcheck/internal/config"
	"mcpcheck/internal/llm"
	"mcpcheck/internal/mcp"
)

const (
	// displayToolResultLimit 是推送给界面的工具结果上限。
	// 界面侧可折叠/滚动展示，因此给得很大，仅在极端异常（数百 KB）时才截断。
	displayToolResultLimit = 500000
	// llmToolResultLimit 是单条工具结果送入 LLM 的字符上限，
	// 防止大报告挤占模型上下文导致请求失败。
	llmToolResultLimit = 16000
	// historyBudget 是送入 LLM 的历史总字符预算；
	// 超出后从最旧的工具结果开始省略，避免多轮点检累积爆上下文。
	historyBudget = 40000
	// omittedToolResult 是历史修剪后的占位说明。
	omittedToolResult = "（该条历史工具结果过长已省略，完整内容见右侧「MCP 工具调用」面板；如需重新获取可再次调用工具）"
)

// Event 是推送给 UI 的对话事件。
type Event struct {
	Type string `json:"type"` // assistant_delta | tool_call | tool_result | done | error
	Text string `json:"text,omitempty"`
	Name string `json:"name,omitempty"`
	Args string `json:"args,omitempty"`
	// Failed 标记 tool_result 是否为失败结果（供界面区分样式）
	Failed bool `json:"failed,omitempty"`
}

// Session 维护一轮会话的对话历史。
type Session struct {
	cfg     *config.Config
	mcpMgr  *mcp.Manager
	mu      sync.Mutex
	history []llm.Message
}

// New 创建会话。
func New(cfg *config.Config, m *mcp.Manager) *Session {
	return &Session{cfg: cfg, mcpMgr: m}
}

// Reset 清空对话历史（新会话）。
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = nil
}

// SystemPrompt 根据当前配置生成系统提示。
func (s *Session) SystemPrompt() string {
	var b strings.Builder
	b.WriteString("你是「MCP点检助手」，运行在用户本机。你的任务是通过 MCP 工具帮助用户对软件工程目录执行点检，并输出中文点检报告（Markdown 格式）。\n")
	if s.cfg.Workspace != "" {
		b.WriteString("当前工作目录（工作区）为：" + s.cfg.Workspace + " 。当工具需要软件工程目录/路径参数时，默认使用该工作区路径，除非用户明确指定了其他路径。\n")
	}
	b.WriteString("规则：1) 需要点检时调用相应 MCP 工具，不要臆造点检结果；2) 工具返回后，基于真实返回内容整理成结构化报告（问题清单、风险等级、修复建议）；3) 工具调用失败时如实说明错误信息。")
	return b.String()
}

func (s *Session) messages() []llm.Message {
	out := make([]llm.Message, 0, len(s.history)+1)
	out = append(out, llm.Message{Role: "system", Content: s.SystemPrompt()})
	out = append(out, s.history...)
	return out
}

// Send 处理一条用户消息：流式输出、按需调用 MCP 工具，直到产出最终回复。
func (s *Session) Send(ctx context.Context, userText string, emit func(Event)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cfg.APIKey == "" {
		err := fmt.Errorf("尚未配置 API Key，请先在左侧「模型设置」中填写")
		emit(Event{Type: "error", Text: err.Error()})
		return err
	}
	if s.mcpMgr.ConnectedCount() == 0 {
		emit(Event{Type: "error", Text: "当前没有已连接的 MCP 服务器，请检查 mcp.json 配置后点击「重新加载」"})
		return fmt.Errorf("没有已连接的 MCP 服务器")
	}

	s.history = append(s.history, llm.Message{Role: "user", Content: userText})
	client := llm.NewClient(s.cfg.BaseURL, s.cfg.APIKey, s.cfg.Model)

	const maxRounds = 8
	for round := 0; round < maxRounds; round++ {
		s.trimHistory()
		tools := s.mcpMgr.AllTools()
		var streamed strings.Builder
		resp, err := client.ChatStream(ctx, s.messages(), tools, func(delta string) {
			streamed.WriteString(delta)
			emit(Event{Type: "assistant_delta", Text: delta})
		})
		if err != nil {
			emit(Event{Type: "error", Text: err.Error()})
			return err
		}
		if len(resp.ToolCalls) == 0 {
			// 最终回复（已流式输出）
			s.history = append(s.history, llm.Message{Role: "assistant", Content: streamed.String()})
			emit(Event{Type: "done"})
			return nil
		}
		// 记录助手消息（含工具调用）
		asst := llm.Message{Role: "assistant", Content: resp.Content, ToolCalls: resp.ToolCalls}
		s.history = append(s.history, asst)
		for _, tc := range resp.ToolCalls {
			if tc.Function.Name == "" {
				continue
			}
			emit(Event{Type: "tool_call", Name: tc.Function.Name, Args: tc.Function.Arguments})
			var args json.RawMessage
			if tc.Function.Arguments != "" {
				args = json.RawMessage(tc.Function.Arguments)
			}
			result, err := s.mcpMgr.CallTool(ctx, tc.Function.Name, args)
			failed := false
			if err != nil {
				result = "工具调用失败：" + err.Error()
				failed = true
			}
			// 界面拿完整内容（可滚动/展开），LLM 侧按上限保护上下文
			emit(Event{Type: "tool_result", Name: tc.Function.Name, Text: truncateRunes(result, displayToolResultLimit), Failed: failed})
			s.history = append(s.history, llm.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    truncateRunes(result, llmToolResultLimit),
			})
		}
	}
	emit(Event{Type: "error", Text: "工具调用轮次过多，已停止。请尝试把需求描述得更具体。"})
	return fmt.Errorf("工具调用轮次超过上限")
}

// trimHistory 在历史超过预算时，从最旧的工具结果开始省略内容，
// 防止多轮点检累积导致请求超出模型上下文。
func (s *Session) trimHistory() {
	total := 0
	for i := range s.history {
		total += len([]rune(s.history[i].Content))
	}
	if total <= historyBudget {
		return
	}
	for i := len(s.history) - 1; i >= 0; i-- {
		m := &s.history[i]
		if m.Role != "tool" || m.Content == omittedToolResult {
			continue
		}
		total -= len([]rune(m.Content))
		m.Content = omittedToolResult
		if total <= historyBudget*3/4 {
			return
		}
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n…（内容过长已截断）"
}
