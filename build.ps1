# build.ps1 —— 一键构建 MCP点检助手
# 用法：在仓库根目录执行  .\build.ps1
$ErrorActionPreference = "Stop"

$root = $PSScriptRoot
Set-Location $root

# --- 定位 Go 工具链：优先仓库自带 .toolchain\go，其次系统 Go ---
$localGo = Join-Path $root ".toolchain\go\bin"
if (Test-Path (Join-Path $localGo "go.exe")) {
    $env:Path = "$localGo;$env:Path"
    Write-Host "使用自带工具链: $localGo"
} elseif (Get-Command go -ErrorAction SilentlyContinue) {
    Write-Host "使用系统 Go: $((Get-Command go).Source)"
} else {
    Write-Error "未找到 Go 工具链（.toolchain\go 或系统 go）"
    exit 1
}

# 国内网络环境：使用 goproxy.cn 镜像
$env:GOPROXY = "https://goproxy.cn,direct"

$dist = Join-Path $root "dist"
New-Item -ItemType Directory -Force -Path $dist | Out-Null

Write-Host "==> 生成图标资源（SVG -> ICO -> rsrc.syso）"
Push-Location (Join-Path $root "tools\icon")
& go run . -in "mcp-icon.svg" -out (Join-Path $root "internal\app\icon.ico")
$icoExit = $LASTEXITCODE
Pop-Location
if ($icoExit -ne 0) { Write-Warning "ICO 生成失败，改用仓库中的 internal\app\icon.ico" }
& go run github.com/akavel/rsrc@latest -ico (Join-Path $root "internal\app\icon.ico") -o (Join-Path $root "rsrc_windows_amd64.syso")
if ($LASTEXITCODE -ne 0) { Write-Warning "图标嵌入失败，exe 将使用系统默认图标（不影响功能）" }

Write-Host "==> go vet"
& go vet ./...
if ($LASTEXITCODE -ne 0) { Write-Error "go vet 失败"; exit 1 }

Write-Host "==> go test ./..."
& go test ./... -timeout 180s
if ($LASTEXITCODE -ne 0) { Write-Error "测试失败"; exit 1 }

Write-Host "==> 构建主程序"
& go build -ldflags="-s -w" -o (Join-Path $dist "MCP点检助手.exe") .
if ($LASTEXITCODE -ne 0) { Write-Error "构建失败"; exit 1 }

Write-Host "==> 构建 Mock（开发/测试用）"
& go build -ldflags="-s -w" -o (Join-Path $dist "mock.exe") ./cmd/mock
if ($LASTEXITCODE -ne 0) { Write-Warning "mock 构建失败（不影响主程序）" }

$exe = Get-Item (Join-Path $dist "MCP点检助手.exe")
Write-Host ("完成: {0} ({1:N2} MB)" -f $exe.FullName, ($exe.Length / 1MB)) -ForegroundColor Green
