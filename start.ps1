# 一键启动后端（PowerShell）
# 用法（仓库根目录）：
#   .\start.ps1              完全按 server\config.yaml 启动（不覆盖任何配置项）
#   .\start.ps1 -Port 8481   只临时覆盖端口，其它仍走 config.yaml
#
# 脚本会先停止残留进程、编译，再后台启动并做健康检查，最后打印实际生效的配置。

param(
    [int]$Port = 0        # 0 = 不覆盖，使用 config.yaml 里的 server.port
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$serverDir = Join-Path $root 'server'
$configPath = Join-Path $serverDir 'config.yaml'

if (-not (Test-Path (Join-Path $serverDir 'main.go'))) {
    Write-Host "找不到 $serverDir\main.go，请在仓库根目录运行本脚本" -ForegroundColor Red
    exit 1
}

# ---------- 1) 读取 config.yaml 里实际生效的配置（仅用于展示与健康检查） ----------
function Get-YamlValue {
    param([string]$Path, [string]$Key)
    if (-not (Test-Path $Path)) { return '' }
    $m = Select-String -Path $Path -Pattern ('^\s*' + [regex]::Escape($Key) + ':\s*([^\s#]+)') | Select-Object -First 1
    if ($m) { return $m.Matches[0].Groups[1].Value }
    return ''
}

$cfgPort   = Get-YamlValue -Path $configPath -Key 'port'
$cfgMode   = Get-YamlValue -Path $configPath -Key 'mode'
$cfgDriver = Get-YamlValue -Path $configPath -Key 'driver'
$cfgDbHost = Get-YamlValue -Path $configPath -Key 'host'
$cfgDbName = Get-YamlValue -Path $configPath -Key 'name'
$cfgRedis  = Get-YamlValue -Path $configPath -Key 'enabled'
if ([string]::IsNullOrWhiteSpace($cfgPort)) { $cfgPort = '8480' }

# 环境变量优先级高于 config.yaml（后端就是这么设计的），这里如实反映
$effectivePort = if ($Port -gt 0) { "$Port" } elseif ($env:KEFU_PORT) { $env:KEFU_PORT } else { $cfgPort }
$portSource = if ($Port -gt 0) { '脚本 -Port 参数' } elseif ($env:KEFU_PORT) { '环境变量 KEFU_PORT' } else { 'config.yaml' }

Write-Host ""
Write-Host "配置来源：$configPath" -ForegroundColor DarkGray
Write-Host ("  端口={0}（来自 {1}）  mode={2}  driver={3}  db={4}/{5}  redis={6}" -f `
    $effectivePort, $portSource, $cfgMode, $cfgDriver, $cfgDbHost, $cfgDbName, $cfgRedis) -ForegroundColor DarkGray
Write-Host ""

# ---------- 2) 停掉残留进程 ----------
$running = Get-Process -Name 'kefu-server', 'go' -ErrorAction SilentlyContinue
if ($running) {
    Write-Host "发现已有进程，先停止：" -ForegroundColor Yellow
    $running | ForEach-Object { Write-Host ("  PID={0} {1}" -f $_.Id, $_.ProcessName) }
    $running | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}

$occupied = Get-NetTCPConnection -LocalPort ([int]$effectivePort) -State Listen -ErrorAction SilentlyContinue
if ($occupied) {
    Write-Host "端口 $effectivePort 仍被 PID $($occupied.OwningProcess) 占用，请先结束该进程" -ForegroundColor Red
    exit 1
}

# ---------- 3) 编译 ----------
$exe = Join-Path $serverDir 'kefu-server.exe'
$tmpExe = Join-Path $serverDir 'kefu-server.new.exe'
Write-Host "正在编译…" -ForegroundColor DarkGray
Push-Location $serverDir
$buildOut = & go build -ldflags="-s -w" -o $tmpExe . 2>&1
$buildCode = $LASTEXITCODE
Pop-Location
if ($buildCode -ne 0 -or -not (Test-Path $tmpExe)) {
    Write-Host "编译失败（exit=$buildCode）：" -ForegroundColor Red
    $buildOut | Select-Object -First 20 | ForEach-Object { Write-Host "  $_" }
    exit 1
}
# 先编译到临时文件再覆盖：Windows 上正在运行的 exe 不能被覆写，旧版会锁住目标文件
Move-Item -Path $tmpExe -Destination $exe -Force

# ---------- 4) 启动（只有显式 -Port 才设置 KEFU_PORT） ----------
if ($Port -gt 0) { $env:KEFU_PORT = "$Port" } else { Remove-Item Env:KEFU_PORT -ErrorAction SilentlyContinue }

$logDir = Join-Path $serverDir 'logs'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$outLog = Join-Path $logDir 'app.log'
$errLog = Join-Path $logDir 'err.log'

Write-Host "正在启动后端…" -ForegroundColor Cyan
Start-Process -FilePath $exe -WorkingDirectory $serverDir `
    -RedirectStandardOutput $outLog -RedirectStandardError $errLog -WindowStyle Hidden

# ---------- 5) 健康检查 ----------
$ok = $false
for ($i = 0; $i -lt 60; $i++) {
    Start-Sleep -Seconds 1
    try {
        $resp = Invoke-RestMethod -Uri "http://127.0.0.1:$effectivePort/health" -TimeoutSec 2 -ErrorAction Stop
        if ($resp.code -eq 0) { $ok = $true; break }
    } catch { }
}

Write-Host ""
if ($ok) {
    Write-Host "启动成功（配置已按上面所示的来源生效）。" -ForegroundColor Green
    Write-Host ""
    Write-Host "  管理后台   http://127.0.0.1:$effectivePort/admin/      （admin / admin123）"
    Write-Host "  客服工作台 http://127.0.0.1:$effectivePort/chat/       （kf01 / 123456）"
    Write-Host "  用户端     http://127.0.0.1:$effectivePort/kefu/?channel=wechat&uid=demo1"
    Write-Host "  健康检查   http://127.0.0.1:$effectivePort/health"
    Write-Host ""
    Write-Host "  根路径 / 是空白页，属设计如此，请使用上面三个入口。"
    Write-Host "  日志：$outLog    停止：.\stop.ps1"
    Write-Host ""
    Write-Host "  修改 config.yaml 后需重新执行本脚本才生效（配置只在启动时读取）。" -ForegroundColor DarkGray
} else {
    Write-Host "启动失败，日志末尾：" -ForegroundColor Red
    if (Test-Path $errLog) { Get-Content $errLog -Tail 30 | ForEach-Object { Write-Host "  $_" } }
    if (Test-Path $outLog) { Get-Content $outLog -Tail 15 | ForEach-Object { Write-Host "  $_" } }
    exit 1
}
