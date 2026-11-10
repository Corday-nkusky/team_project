@echo off
rem ASCII only on purpose: cmd.exe mis-parses UTF-8 Chinese lines under chcp 65001.
rem All Chinese messages live in scripts/stop.mjs (Node prints UTF-8).
chcp 65001 >nul 2>nul
cd /d "%~dp0"
set "NODE_EXE="
for /f "delims=" %%i in ('where node 2^>nul') do if not defined NODE_EXE set "NODE_EXE=%%i"
if not defined NODE_EXE if exist "%ProgramFiles%\nodejs\node.exe" set "NODE_EXE=%ProgramFiles%\nodejs\node.exe"
if not defined NODE_EXE (
  for /d %%d in ("%USERPROFILE%\.workbuddy\binaries\node\versions\*") do (
    if exist "%%d\node.exe" set "NODE_EXE=%%d\node.exe"
  )
)
if not defined NODE_EXE (
  echo [ERROR] Node.js 20+ not found.
  echo.
  echo Install it with one of these, then double-click this file again:
  echo    1. winget install OpenJS.NodeJS.LTS
  echo    2. https://nodejs.org  - LTS installer, keep "Add to PATH" checked
  echo.
  echo On macOS / Linux this .cmd does not apply. Run instead: node scripts/stop.mjs
  echo.
  pause
  exit /b 1
)
echo Using Node: %NODE_EXE%
echo.
"%NODE_EXE%" "scripts\stop.mjs" %*
set "RC=%ERRORLEVEL%"
echo.
echo Press any key to close this window.
pause >nul
exit /b %RC%
