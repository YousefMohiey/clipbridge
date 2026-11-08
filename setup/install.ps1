$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "  ClipBridge setup" -ForegroundColor Cyan
Write-Host "  ----------------"
Write-Host ""

$src = Join-Path $PSScriptRoot "clipbridge.ps1"
if (-not (Test-Path $src)) {
    Write-Host "  clipbridge.ps1 is missing next to this installer." -ForegroundColor Red
    exit 1
}

$dir = Join-Path $env:LOCALAPPDATA "ClipBridge"
New-Item -ItemType Directory -Force $dir | Out-Null
Copy-Item $src (Join-Path $dir "clipbridge.ps1") -Force
Write-Host "  copied client to $dir"

$arg = '-WindowStyle Hidden -STA -ExecutionPolicy Bypass -File "' + (Join-Path $dir "clipbridge.ps1") + '"'
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument $arg
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
Register-ScheduledTask -TaskName "ClipBridge" -Action $action -Trigger $trigger -Settings $settings -Description "Clipboard bridge to the Linux host" -Force | Out-Null
Start-ScheduledTask -TaskName "ClipBridge"
Write-Host "  registered the ClipBridge task"

Start-Sleep -Seconds 3

$ok = $false
try {
    $health = Invoke-RestMethod "http://192.168.1.8:1717/health" -TimeoutSec 5
    $ok = [bool]$health.ok
} catch { }

Write-Host ""
if ($ok) {
    Write-Host "  Done. Clipboard sync is running in the background." -ForegroundColor Green
    Write-Host "  Copy text or a screenshot on either machine and paste on the other."
} else {
    Write-Host "  Installed, but the laptop did not answer on port 1717." -ForegroundColor Yellow
    Write-Host "  Check that the laptop is awake and on the same network."
}
Write-Host ""
Write-Host "  Log: $dir\clipbridge.log"
Write-Host "  Remove: run Uninstall ClipBridge.cmd"
Write-Host ""
