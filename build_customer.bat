@echo off
REM Build the CUSTOMER edition (core info hidden). Two modes:
REM   build_customer.bat          -> PLAIN build (default). Ships to customers and runs directly.
REM   build_customer.bat garble   -> OBFUSCATED build. Hides string constants (relay URL) too,
REM                                  BUT Windows Defender usually flags garble output as a virus,
REM                                  so it is NOT suitable for direct distribution unless you
REM                                  ALSO code-sign the exe. Use only if you have a signing cert.
REM
REM Both modes hide core info at runtime (UI / API / on-disk config.json / logs).
REM The FULL edition still uses build.bat (Edition defaults to full).
setlocal
cd /d "%~dp0"
set GOTOOLCHAIN=local
set "LDFLAGS=-s -w -H=windowsgui -X bailan-relay/internal/config.Edition=customer"
set "OUT=bailan-relay-customer.exe"

REM --- 定位 go：PATH 找不到时回退常见安装目录 ---
where go >nul 2>nul
if errorlevel 1 if exist "C:\Program Files\Go\bin\go.exe"                 set "PATH=C:\Program Files\Go\bin;%PATH%"
if errorlevel 1 if exist "C:\Go\bin\go.exe"                               set "PATH=C:\Go\bin;%PATH%"
if errorlevel 1 if exist "D:\go\bin\go.exe"                               set "PATH=D:\go\bin;%PATH%"
if errorlevel 1 if exist "%LOCALAPPDATA%\Programs\Go\bin\go.exe"          set "PATH=%LOCALAPPDATA%\Programs\Go\bin;%PATH%"
if errorlevel 1 if exist "%USERPROFILE%\scoop\apps\go\current\bin\go.exe" set "PATH=%USERPROFILE%\scoop\apps\go\current\bin;%PATH%"
if errorlevel 1 if exist "%USERPROFILE%\go\bin\go.exe"                    set "PATH=%USERPROFILE%\go\bin;%PATH%"
where go >nul 2>nul
if errorlevel 1 (
    echo [x] 找不到 go.exe：请先安装 Go，或把 Go 的 bin 目录加入 PATH 后重试。
    exit /b 1
)

if /i "%~1"=="garble" goto :garble

echo [1/1] Building customer edition (plain, distributable) ...
go build -trimpath -ldflags "%LDFLAGS%" -o "%OUT%" .
if errorlevel 1 goto :err
echo.
echo Customer build done: %~dp0%OUT%
echo This is a normal exe: send it to customers and run as Administrator.
echo Note: a technical user could still recover strings (e.g. relay URL) from the exe.
echo       For maximum protection, build with "garble" AND code-sign the result.
goto :eof

:garble
for /f "delims=" %%i in ('go env GOPATH') do set "GARBLE=%%i\bin\garble.exe"
if not exist "%GARBLE%" (
  echo [x] garble not found. Install: set GOTOOLCHAIN=local ^&^& set GOPROXY=https://goproxy.cn,direct ^&^& go install mvdan.cc/garble@v0.15.0
  exit /b 1
)
echo [1/1] Building customer edition (garble -literals, obfuscated) ...
echo [!] WARNING: Windows Defender often deletes garble output as a false positive.
echo     Do NOT distribute this unsigned. Code-sign the exe before sending to customers.
"%GARBLE%" -literals build -trimpath -ldflags "%LDFLAGS%" -o "%OUT%" .
if errorlevel 1 goto :err
echo.
echo Obfuscated customer build done: %~dp0%OUT%  ^(code-sign before distributing^)
goto :eof

:err
echo Customer build failed.
exit /b 1
