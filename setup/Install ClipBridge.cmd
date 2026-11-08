@echo off
cd /d "%~dp0"
echo.
echo  Installing ClipBridge...
echo.
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1"
echo.
pause
