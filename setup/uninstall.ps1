$ErrorActionPreference = "SilentlyContinue"
Stop-ScheduledTask -TaskName "ClipBridge"
Unregister-ScheduledTask -TaskName "ClipBridge" -Confirm:$false
Get-Process | Where-Object { $_.Path -like "*\ClipBridge\clipbridge.ps1*" } | Stop-Process -Force
Write-Host ""
Write-Host "  ClipBridge removed." -ForegroundColor Green
Write-Host "  The folder $env:LOCALAPPDATA\ClipBridge was left in place with its log."
Write-Host ""
