@echo off
setlocal
if not defined LOCALAPPDATA (
  echo Windows LOCALAPPDATA is unavailable. Preview was not started.
  exit /b 1
)
if not exist "%~dp0myco-app.exe" exit /b 1
if not exist "%~dp0myco.exe" exit /b 1
if not exist "%~dp0component\0.1.0\myco-remote.exe" exit /b 1
if not exist "%~dp0component\0.1.0\ffmpeg.exe" exit /b 1
set "RemotePreviewRoot=%LOCALAPPDATA%\myco-remote-preview\desktop"
for /f "tokens=1 delims==" %%V in ('set MYCO_ 2^>nul') do set "%%V="
set "USERPROFILE=%RemotePreviewRoot%\profile"
set "HOME=%USERPROFILE%"
set "APPDATA=%USERPROFILE%\AppData\Roaming"
set "LOCALAPPDATA=%USERPROFILE%\AppData\Local"
set "PATH=%~dp0;%PATH%"
set "MYCO_APP_CACHE=off"
set "MYCO_NO_UPDATE_CHECK=1"
if not exist "%APPDATA%" mkdir "%APPDATA%"
if not exist "%LOCALAPPDATA%" mkdir "%LOCALAPPDATA%"
if not exist "%USERPROFILE%\.myco\components\remote\0.1.0" mkdir "%USERPROFILE%\.myco\components\remote\0.1.0"
xcopy /E /I /Y "%~dp0component\0.1.0" "%USERPROFILE%\.myco\components\remote\0.1.0" >nul
if errorlevel 1 (
  echo Could not prepare preview components. Close this preview and try again.
  exit /b 1
)
if /I "%~1"=="--check" (
  set USERPROFILE
  set HOME
  set APPDATA
  set LOCALAPPDATA
  set MYCO_
  "%~dp0myco.exe" --version
  "%~dp0myco-app.exe" --version
  exit /b
)
start "" /D "%~dp0" "%~dp0myco-app.exe"
