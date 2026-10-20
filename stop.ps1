# 一键停止后端（PowerShell）
# 用法：在仓库根目录执行  .\stop.ps1

$running = Get-Process -Name 'kefu-server', 'go' -ErrorAction SilentlyContinue
if (-not $running) {
    Write-Host "没有正在运行的后端进程。" -ForegroundColor Yellow
} else {
    $running | ForEach-Object { Write-Host ("  停止 PID={0} {1}" -f $_.Id, $_.ProcessName) }
    $running | Stop-Process -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    Write-Host "已停止。" -ForegroundColor Green
}

# 顺带确认端口释放
$occupied = Get-NetTCPConnection -LocalPort 8480 -State Listen -ErrorAction SilentlyContinue
if ($occupied) {
    Write-Host "注意：8480 仍被 PID $($occupied.OwningProcess) 占用（可能是其它程序）" -ForegroundColor Yellow
}
