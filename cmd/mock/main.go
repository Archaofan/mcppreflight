// cmd/mock 提供本地 Mock 服务器，用于无人值守测试：
//   - 模拟 DeepSeek（OpenAI 兼容）API：:9811
//   - 模拟 MCP Server（Streamable HTTP / SSE）：:9812
// 不参与正式发布，仅用于开发测试。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
)

var (
	dsPort  = flag.Int("ds", 9811, "Mock DeepSeek 端口")
	mcpPort = flag.Int("mcp", 9812, "Mock MCP 端口")
	asSSE   = flag.Bool("sse", false, "MCP 响应使用 SSE 流格式")
)

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func main() {
	flag.Parse()
	mux := http.NewServeMux()

	// ---------- Mock DeepSeek ----------
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role       string `json:"role"`
				Content    string `json:"content"`
				ToolCalls  []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		hasToolResult := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				hasToolResult = true
			}
		}
		log.Printf("[mock-ds] model=%s stream=%v msgs=%d hasToolResult=%v", body.Model, body.Stream, len(body.Messages), hasToolResult)

		if !hasToolResult {
			// 第一轮：以 SSE 流式返回一个 tool_call
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			flusher := w.(http.Flusher)
			argParts := []string{`{"path"`, `:"E:\\DSH-Workspace\\MCP-tool"}`}
			for i, part := range argParts {
				chunk := map[string]interface{}{
					"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{
						"tool_calls": []map[string]interface{}{{
							"index": 0,
							"id":    "call_mock_001",
							"type":  "function",
							"function": map[string]string{
								"name":      "scan_project",
								"arguments": part,
							},
						}},
					}}},
				}
				if i > 0 {
					// 第二片不再重复 id/name
					chunk["choices"].([]map[string]interface{})[0]["delta"].(map[string]interface{})["tool_calls"].([]map[string]interface{})[0]["id"] = ""
					chunk["choices"].([]map[string]interface{})[0]["delta"].(map[string]interface{})["tool_calls"].([]map[string]interface{})[0]["function"].(map[string]string)["name"] = ""
				}
				b, _ := json.Marshal(chunk)
				fmt.Fprintf(w, "data: %s\n\n", b)
				flusher.Flush()
			}
			done, _ := json.Marshal(map[string]interface{}{
				"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{}, "finish_reason": "tool_calls"}},
			})
			fmt.Fprintf(w, "data: %s\n\n", done)
			fmt.Fprint(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
		// 第二轮：流式输出最终报告
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher := w.(http.Flusher)
		report := "# 点检报告\n\n## 概况\n工作区共 **128** 个文件，扫描耗时 **3.2s**。\n\n## 发现的问题\n1. **高危**：`config.json` 中包含明文 API Key\n2. **中危**：依赖 `golang.org/x/sys` 版本过旧\n3. **低危**：存在 3 个 TODO 未处理\n\n## 建议\n- 立即轮换 API Key 并改用环境变量\n- 执行 `go get -u` 升级依赖\n\n```bash\ngo test ./...\n```\n\n> 本报告由 MCP 点检工具生成。"
		chunks := splitRunes(report, 12)
		for _, ch := range chunks {
			b, _ := json.Marshal(map[string]interface{}{
				"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": ch}}},
			})
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "deepseek-chat"}, {"id": "deepseek-reasoner"}},
		})
	})

	// ---------- Mock MCP ----------
	var sessionSeq int64
	mcpHandler := func(w http.ResponseWriter, r *http.Request) {
		var req rpcReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		log.Printf("[mock-mcp] method=%s id=%d sse=%v", req.Method, req.ID, *asSSE)
		switch req.Method {
		case "initialize":
			sid := atomic.AddInt64(&sessionSeq, 1)
			w.Header().Set("Mcp-Session-Id", fmt.Sprintf("mock-session-%d", sid))
			writeRPC(w, req.ID, map[string]interface{}{
				"protocolVersion": "2025-06-18",
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]string{"name": "mock-dianjian", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPC(w, req.ID, map[string]interface{}{
				"tools": []map[string]interface{}{{
					"name":        "scan_project",
					"description": "对指定软件工程目录执行点检并输出报告（需要 path 参数）",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"path": map[string]string{"type": "string", "description": "软件工程目录绝对路径"},
						},
						"required": []string{"path"},
					},
				}},
			})
		case "tools/call":
			var params struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			log.Printf("[mock-mcp] tools/call name=%s args=%v", params.Name, params.Arguments)
			// 模拟一份较大的点检报告（约 9000 字），用于验证大结果处理
			var sb strings.Builder
			sb.WriteString("点检完成：共扫描 128 个文件；发现 3 个问题（1 高危 / 1 中危 / 1 低危）；报告已生成。\n\n")
			sb.WriteString("================ 详细点检明细 ================\n")
			for i := 1; i <= 120; i++ {
				sb.WriteString(fmt.Sprintf("【条目 %03d】路径：src/module%02d/file%02d.go  状态：正常  说明：静态扫描未发现问题，符合编码规范。\n", i, i%20+1, i%30+1))
			}
			sb.WriteString("================ 报告结束 ================\n")
			writeRPC(w, req.ID, map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": sb.String()}},
			})
		default:
			writeRPCError(w, req.ID, -32601, "method not found: "+req.Method)
		}
	}
	mux.HandleFunc("/mcp", mcpHandler)

	addr1 := fmt.Sprintf("127.0.0.1:%d", *dsPort)
	addr2 := fmt.Sprintf("127.0.0.1:%d", *mcpPort)
	log.Printf("Mock DeepSeek: http://%s", addr1)
	log.Printf("Mock MCP:      http://%s/mcp (sse=%v)", addr2, *asSSE)
	go func() { log.Fatal(http.ListenAndServe(addr1, mux)) }()
	log.Fatal(http.ListenAndServe(addr2, mux))
}

func writeRPC(w http.ResponseWriter, id int, result interface{}) {
	b, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": id, "result": result,
	})
	if *asSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func writeRPCError(w http.ResponseWriter, id int, code int, msg string) {
	b, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]interface{}{"code": code, "message": msg},
	})
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

func splitRunes(s string, n int) []string {
	r := []rune(s)
	var out []string
	for i := 0; i < len(r); i += n {
		end := i + n
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[i:end]))
	}
	_ = strings.TrimSpace
	return out
}
