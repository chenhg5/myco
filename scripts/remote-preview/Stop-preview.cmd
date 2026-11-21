@echo off
setlocal
set "RemotePreviewExe=%~dp0myco-app.exe"
powershell.exe -NoProfile -NonInteractive -Command "Get-Process myco-app -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $env:RemotePreviewExe } | Stop-Process"
