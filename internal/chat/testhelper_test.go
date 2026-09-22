package chat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newFakeLLM 模拟 DeepSeek：首轮 tool_call，次轮流式报告。
func newFakeLLM(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		hasTool := false
		for _, m := range body.Messages {
			if m.Role == "tool" {
				hasTool = true
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		send := func(v interface{}) {
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
		if !hasTool {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]interface{}{
				"tool_calls": []map[string]interface{}{{"index": 0, "id": "call_x", "type": "function",
					"function": map[string]string{"name": "scan_project", "arguments": `{"path":"E:\\test"}`}}},
			}}}})
		} else {
			send(map[string]interface{}{"choices": []map[string]interface{}{{"index": 0, "delta": map[string]string{"content": "点检报告：一切正常"}}}})
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
