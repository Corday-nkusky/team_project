@echo off
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
  echo [ERROR] Node.js not found. Install Node.js 20+ or add it to PATH.
  echo.
  pause
  exit /b 1
)
echo Using Node: %NODE_EXE%
echo.
"%NODE_EXE%" "scripts\launch.mjs" %*
set "RC=%ERRORLEVEL%"
set "IS_CHECK="
echo %* | findstr /C:"--check" >nul && set "IS_CHECK=1"
echo.
if "%RC%"=="0" (
  if defined IS_CHECK (
    echo [OK] Check finished. Nothing was started.
  ) else (
    echo [OK] CampusClaw is up: http://127.0.0.1:8080
  )
) else (
  echo [FAILED] exit code %RC%. Read the messages above.
)
echo.
echo Press any key to close this window.
pause >nul
exit /b %RC%
