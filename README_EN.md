# MCP Preflight (MCP点检助手)

[简体中文](README.md) ｜ **English**

A minimal Windows desktop tool: let an LLM (DeepSeek by default, or any of 12 other providers) call your MCP inspection tools against a source tree and produce a report — right inside a chat window.

A single ~8 MB statically-linked Go exe. **No .NET / Node / Python / VC++ runtime required** — double-click on Windows 10 / 11.

![Screenshot (light theme)](docs/screenshot.png)

---

## Features

- **Single-file portable exe**: copy it and run it — no installer, no registry writes
- **Multi-provider**: DeepSeek / Aliyun DashScope (Qwen) / Zhipu BigModel (GLM) / Moonshot (Kimi) / iFlytek Spark / MiniMax / SiliconFlow / Volcano Ark (Doubao) / Baidu Qianfan (ERNIE) / Tencent Hunyuan / Ollama (local) / OpenRouter, plus **Custom** for any OpenAI-compatible endpoint (one-api / new-api relays included)
- **Three-column UI**: config on the left, conversation + report in the middle, live **MCP tool calls** panel on the right (args / result / status — pale yellow = args, pale green = result, pale red = failure)
- **Full tool results kept**: long results are collapsed by default and expandable, with the total character count shown; a separate length guard applies to what is sent to the model
- **Bilingual UI (zh-CN / en) + light / dark theme**: switch any time from the Appearance section, persisted in `config.json`
- **http only**: MCP over Streamable HTTP or SSE; stdio servers are explicitly marked unsupported
- **Offline-friendly**: no external CDN resources, local service bound to `127.0.0.1` only, no telemetry

## Supported providers

| Provider | Base URL | Tool calling | Notes |
| --- | --- | --- | --- |
| **DeepSeek** (default) | `https://api.deepseek.com` | ✅ full | no `/v1` suffix; default model `deepseek-flash` |
| Aliyun DashScope (Qwen) | `https://dashscope.aliyuncs.com/compatible-mode/v1` | ✅ full | API key is region-bound |
| Zhipu BigModel (GLM) | `https://open.bigmodel.cn/api/paas/v4` | ✅ full | `tool_choice` only accepts `auto` |
| Moonshot (Kimi) | `https://api.moonshot.cn/v1` | ✅ full | k2.x accepts `auto`/`none` only |
| iFlytek Spark | `https://spark-api-open.xf-yun.com/v1` | ⚠️ partial | sends `tool_calls_switch=true` automatically |
| MiniMax | `https://api.minimax.cn/v1` | ✅ full | thinking cannot be disabled on M2.x |
| SiliconFlow | `https://api.siliconflow.com/v1` | ⚠️ partial | sends `enable_thinking=false` automatically |
| Volcano Ark (Doubao) | `https://ark.cn-beijing.volces.com/api/v3` | ⚠️ partial | model may be an `ep-` endpoint ID; enable the model first |
| Baidu Qianfan (ERNIE) | `https://qianfan.baidubce.com/v2` | ⚠️ partial | requires v2 + a static API key |
| Tencent Hunyuan | `https://api.hunyuan.cloud.tencent.com/v1` | ⚠️ partial | tools on turbos/t1/functioncall only |
| Ollama (local) | `http://localhost:11434/v1` | ✅ full | key optional; `tool_choice` is not sent |
| OpenRouter | `https://openrouter.ai/api/v1` | ✅ full | models use the `vendor/model` format |
| Custom | typed manually | depends | one-api / new-api relays |

Every Base URL and model list was verified character-by-character against the official documentation. Adding a provider means adding one row to the preset table in `internal/config/providers.go`.

## Quick start

```powershell
# build (Go 1.27+; skip installing if you use the bundled .toolchain\go)
powershell -ExecutionPolicy Bypass -File .\build.ps1

# run
dist\MCP点检助手.exe
```

Four steps: **pick a provider → paste your API key → save** → **choose a workspace folder** → **load `mcp.json`** → type an inspection command (Enter to send, Shift+Enter for a newline).

`mcp.json` uses the same format as Cursor:

```json
{
  "mcpServers": {
    "dianjian": {
      "url": "http://192.168.1.10:3000/mcp",
      "headers": { "Authorization": "Bearer your-token" }
    }
  }
}
```

Command line: `--headless "command"` (run one command without the UI), `--serve` (local service only), `--port N` (fixed port), `--config path`.

## Layout

```
main.go                 entry: GUI / --headless / --serve / --port / --config
main_windows.go         WebView2 window, console hiding, browser fallback
internal/
  config/               config.json / mcp.json I/O (UTF-8 BOM tolerant) + provider presets
  mcp/                  MCP client (Streamable HTTP + SSE) and connection manager
  llm/                  OpenAI-compatible client (streaming, tool-call aggregation,
                        reasoning_content round-trip)
  chat/                 session orchestration: system prompt (with workspace) →
                        LLM ⇄ MCP loop (≤8 rounds)
  i18n/                 zh/en message tables (Go-side strings)
  app/                  HTTP API + SSE push + go:embed single-page UI
cmd/mock/               dev mock: fake DeepSeek (:9811) + fake MCP (:9812)
tools/icon/             SVG → multi-size ICO (build-time app icon)
tools/peicon/           PE resource inspector: verifies the exe icon
```

Data flow: `WebView2/browser → http://127.0.0.1:PORT → REST/SSE → chat.Session → (llm ⇄ mcp.Manager)`

## Testing

```powershell
$env:Path = ".toolchain\go\bin;$env:Path"
$env:GOPROXY = "https://goproxy.cn,direct"
go vet ./...
go test ./...                                  # unit + integration (httptest fake MCP / fake DeepSeek)
node dist\gen_panel_test.js                    # generate the frontend logic test
node "$env:TEMP\paneltest.js"                  # run it (i18n / theme / providers / panel / drag)
```

End-to-end without a real API key:

```powershell
.\dist\mock.exe &
.\dist\MCP点检助手.exe --headless "inspect the workspace" --config .\dist\e2e\config.json
```

Key regressions covered: provider preset integrity and character-exact Base URLs, i18n completeness, `/api/providers`, config round-trip and invalid-value rejection, `reasoning_content` round-trip, provider-specific body parameters, Ollama capability degradation, right-panel structure and width dragging.

## Known limitations

- MCP `command` (stdio) servers are unsupported — the minimal desktop build ships no Node/Python runtime, and the UI says so.
- Multi-step / parallel tool calls are not guaranteed: the orchestration loop runs at most 8 rounds; keep chatting if a step needs refinement.
- Models change fast — the list fetched via **Fetch** is whatever the provider returns live; some providers expose no model-list API and the tool says so.
- Not code-signed: the first run shows a SmartScreen prompt (**More info** → **Run anyway**).

## Related

- [使用说明.md](使用说明.md) — user guide (Chinese)
- [README.md](README.md) — 简体中文

## License

[MIT](LICENSE) © 2026 Archaofan
