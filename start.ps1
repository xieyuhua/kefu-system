# 一键启动后端（PowerShell）
# 用法：右键「使用 PowerShell 运行」，或在仓库根目录执行  .\start.ps1
# 端口可用 -Port 覆盖，例如  .\start.ps1 -Port 8481

param(
    [int]$Port = 8480
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$serverDir = Join-Path $root 'server'

if (-not (Test-Path (Join-Path $serverDir 'main.go'))) {
    Write-Host "找不到 $serverDir\main.go，请在仓库根目录运行本脚本" -ForegroundColor Red
    exit 1
}

# 1) 先清掉可能残留的旧实例，避免「端口被占用」
$running = Get-Process -Name 'kefu-server', 'go' -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "发现已有进程，先停止：" -ForegroundColor Yellow
    $running | ForEach-Object { Write-Host ("  PID={0} {1}" -f $_.Id, $_.ProcessName) }
    $running | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

# 2) 端口占用兜底检查
$occupied = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($occupied) {
    Write-Host "端口 $Port 仍被 PID $($occupied.OwningProcess) 占用，请手动结束该进程后重试" -ForegroundColor Red
    exit 1
}

# 3) 前端产物缺失时提示（页面会空白/404）
if (-not (Test-Path (Join-Path $serverDir 'web\index.html'))) {
    Write-Host "提示：server\web 不存在，请先在 web 目录执行 pnpm install && pnpm build" -ForegroundColor Yellow
}

# 4) 启动
Write-Host ""
Write-Host "正在启动后端（端口 $Port）…" -ForegroundColor Cyan
$env:KEFU_PORT = "$Port"
$logDir = Join-Path $serverDir 'logs'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$outLog = Join-Path $logDir 'app.log'
$errLog = Join-Path $logDir 'err.log'

# 先编译成二进制再后台启动：避免 `go run` 派生的孙进程占住终端句柄（脚本会看起来卡住不返回）
$exe = Join-Path $serverDir 'kefu-server.exe'
Write-Host "正在编译…" -ForegroundColor DarkGray
Push-Location $serverDir
& go build -ldflags="-s -w" -o $exe . 2>&1 | Out-Null
Pop-Location
if (-not (Test-Path $exe)) {
    Write-Host "编译失败，请检查 Go 环境（go version）" -ForegroundColor Red
    exit 1
}

Start-Process -FilePath $exe -WorkingDirectory $serverDir `
    -RedirectStandardOutput $outLog -RedirectStandardError $errLog -WindowStyle Hidden

# 5) 等待健康检查通过（最多 60 秒）
$ok = $false
for ($i = 0; $i -lt 60; $i++) {
    Start-Sleep -Seconds 1
    try {
        $resp = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/health" -TimeoutSec 2 -ErrorAction Stop
        if ($resp.code -eq 0) { $ok = $true; break }
    } catch { }
}

Write-Host ""
if ($ok) {
    Write-Host "启动成功。" -ForegroundColor Green
    Write-Host ""
    Write-Host "  管理后台   http://127.0.0.1:$Port/admin/      （admin / admin123）"
    Write-Host "  客服工作台 http://127.0.0.1:$Port/chat/       （kf01 / 123456）"
    Write-Host "  用户端     http://127.0.0.1:$Port/kefu/?channel=wechat&uid=demo1"
    Write-Host "  健康检查   http://127.0.0.1:$Port/health"
    Write-Host ""
    Write-Host "  根路径 / 是空白页，属设计如此，请使用上面三个入口。"
    Write-Host "  日志：$outLog"
    Write-Host "  停止：.\stop.ps1"
} else {
    Write-Host "启动失败，错误日志（$errLog）末尾：" -ForegroundColor Red
    if (Test-Path $errLog) { Get-Content $errLog -Tail 30 | ForEach-Object { Write-Host "  $_" } }
    if (Test-Path $outLog) { Get-Content $outLog -Tail 10 | ForEach-Object { Write-Host "  $_" } }
    exit 1
}
