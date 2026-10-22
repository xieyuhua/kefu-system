# 启动压测控制台（自动编译并在浏览器打开）
param(
    [int]$Port = 8600,
    [switch]$Rebuild
)

$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$exe = Join-Path $PSScriptRoot 'loadtest.exe'
if ($Rebuild -or -not (Test-Path $exe)) {
    Write-Host '正在编译压测台…'
    go build -o $exe .
    if ($LASTEXITCODE -ne 0) { throw '编译失败，请检查 Go 环境' }
}

$busy = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($busy) {
    Write-Host "端口 $Port 已被占用，请换端口：.\start-loadtest.ps1 -Port $($Port + 1)"
    exit 1
}

Start-Process -FilePath $exe -ArgumentList @('-port', $Port) -WorkingDirectory $PSScriptRoot
Start-Sleep -Milliseconds 1200

Write-Host "压测控制台已启动：http://127.0.0.1:$Port/"
Write-Host '关闭控制台：在运行窗口按 Ctrl+C，或停止 loadtest.exe 进程'
Start-Process "http://127.0.0.1:$Port/"
