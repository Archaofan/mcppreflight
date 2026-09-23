# MCP点检助手

极简 Windows 桌面工具：在聊天界面中通过 DeepSeek（OpenAI 兼容 API）调用 MCP 点检工具，对软件工程目录执行点检并输出报告。

- **单文件 exe**（约 8 MB，Go 静态编译，无运行时依赖），Win10/Win11 直接双击运行
- **GUI 开箱即用**：WebView2 渲染的本地界面（未装 WebView2 时自动回退系统浏览器），三栏布局：左侧配置、中间对话与报告、右侧「MCP 工具调用」面板（参数/返回/状态实时可见）
- **零手写 JSON 之外的配置**：界面内完成 API Key、模型、工作区、mcp.json 全部配置
- **http only**：MCP 新服务器支持 Streamable HTTP / SSE 两种传输，stdio 类型明确标注「暂不支持」
- 本地 API 服务仅监听 `127.0.0.1`，无外网监听、无遥测

> 使用者视角的说明见 [使用说明.md](使用说明.md)；本文档面向开发者。

## 快速开始

```powershell
# 1. 构建（需要 Go 1.27+；本仓库自带 .toolchain\go 时可省略安装）
powershell -ExecutionPolicy Bypass -File .\build.ps1

# 2. 运行
dist\MCP点检助手.exe
```

启动后：填写 DeepSeek API Key → 保存 → 选择工作区文件夹 → 加载 `mcp.json` → 输入点检指令。

## 架构

```
main.go                 入口：GUI / --headless / --serve / --port / --config
main_windows.go         WebView2 窗口、控制台隐藏、浏览器兜底
internal/
  config/               config.json / mcp.json 读写（兼容 UTF-8 BOM）
  mcp/                  MCP 客户端（Streamable HTTP + SSE）与连接管理器
  llm/                  DeepSeek OpenAI 兼容客户端（始终流式、工具调用聚合）
  chat/                 会话编排：系统提示（含工作区）→ LLM ⇄ MCP 循环（≤8 轮）
  app/                  HTTP API + SSE 推送 + go:embed 嵌入式单页 UI
cmd/mock/               开发用 Mock：假 DeepSeek（:9811）+ 假 MCP（:9812）
tools/
  icon/                 SVG → 多尺寸 ICO 工具（构建期生成应用图标）
  peicon/               PE 资源检查器：验证 exe 是否嵌入图标
```

数据流：`WebView2/浏览器 → http://127.0.0.1:PORT → REST/SSE → chat.Session → (llm ⇄ mcp.Manager)`

- LLM 侧：`POST {base_url}/chat/completions`，`stream:true`，工具来自 MCP `tools/list` 的 schema
- MCP 侧：`initialize`（协议版本 `2025-06-18` → `2025-03-26` → `2024-11-05` 依次回退）→ `notifications/initialized` → `tools/list` / `tools/call`
- 多服务器工具重名时以 `name@server` 区分

## 开发与测试

```powershell
$env:Path = ".toolchain\go\bin;$env:Path"     # 自带工具链；或使用系统 Go
$env:GOPROXY = "https://goproxy.cn,direct"    # 国内网络
go vet ./...
go test ./...                                 # 单元 + 集成（httptest 假 MCP / 假 DeepSeek）
```

用真实二进制做端到端验证（无需真实 API Key）：

```powershell
.\dist\mock.exe &                              # 启动 Mock DeepSeek + Mock MCP
.\dist\MCP点检助手.exe --headless "请点检工作区" --config .\dist\e2e\config.json
.\dist\MCP点检助手.exe --serve                   # 仅起本地服务，便于 curl 调试
```

## 构建

`build.ps1`：图标生成（SVG → ICO → rsrc.syso）+ `go vet` + `go test ./...` + `go build -ldflags="-s -w"` → `dist\MCP点检助手.exe`（含 `cmd/mock`）。产物约 7.8 MB。
（脚本内含中文，需以 `powershell -ExecutionPolicy Bypass -File .\build.ps1` 方式运行；Windows PowerShell 5.1 不会直接执行未签名的 .ps1。）

发布给同事时只需拷贝 **`dist\MCP点检助手.exe`**（或连同 `mcp.json.example` 改名后的 `mcp.json`），无需安装任何运行时。

### 应用图标

- 源文件：`tools/icon/mcp-icon.svg`（600×600 单色 logo）
- `build.ps1` 中 `tools/icon` 用 `tdewolff/canvas` 把它栅格化为 8 个尺寸（16/20/24/32/40/48/64/256）并打包成 `internal/app/icon.ico`，再由 `akavel/rsrc` 生成 `rsrc_windows_amd64.syso` 嵌入 exe（Go 链接器自动识别同目录 `*.syso`）
- `main_windows.go` 中 `IconId: 1` 对应 rsrc 分配的第一个 RT_GROUP_ICON 资源 ID，WebView2 窗口与任务栏即显示该图标
- 验证嵌入结果：`go run ./tools/peicon <exe>`（解析 PE 资源目录，列出图标组与全部尺寸）

## 已知边界

- MCP `command`（stdio）类型不支持：桌面极简版不含 Node/Python 运行时，界面明确标注。
- DeepSeek 多步/并行工具调用不保证：编排循环最多 8 轮，单步结果不满意可继续对话。
- 未做代码签名：首次运行会有 SmartScreen 提示（点「更多信息」→「仍要运行」）。
