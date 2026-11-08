# Installs ClipBridge as a hidden background task that starts at logon.
# Nothing to keep open afterwards.

$ErrorActionPreference = "Stop"
$dir = Join-Path $env:LOCALAPPDATA "ClipBridge"
New-Item -ItemType Directory -Force $dir | Out-Null

Invoke-WebRequest "http://192.168.1.8:1717/clipbridge.ps1" -OutFile (Join-Path $dir "clipbridge.ps1") -UseBasicParsing

$arg = '-WindowStyle Hidden -STA -ExecutionPolicy Bypass -File "' + (Join-Path $dir "clipbridge.ps1") + '"'
$action = New-ScheduledTaskAction -Execute "powershell.exe" -Argument $arg
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable

Register-ScheduledTask -TaskName "ClipBridge" -Action $action -Trigger $trigger -Settings $settings -Description "Clipboard bridge to the Linux host" -Force | Out-Null
Start-ScheduledTask -TaskName "ClipBridge"

Write-Host ""
Write-Host "ClipBridge installed."
Write-Host "  script : $dir\clipbridge.ps1"
Write-Host "  log    : $dir\clipbridge.log"
Write-Host "  task   : ClipBridge (runs hidden at every logon)"
Write-Host ""
Write-Host "Stop it now      : Stop-ScheduledTask -TaskName ClipBridge"
Write-Host "Remove it fully  : Unregister-ScheduledTask -TaskName ClipBridge -Confirm:`$false"
