@echo off
REM 混淆 + 打包构建：garble 混淆标识符/字符串 + 去符号 + (可选)UPX 压缩。
REM 注意：garble 必须与本机 Go 版本匹配。本项目：go1.25 对应 garble v0.15.0。
REM 安装：set GOTOOLCHAIN=local ^&^& go install mvdan.cc/garble@v0.15.0
setlocal
cd /d "%~dp0"
set GOTOOLCHAIN=local

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

for /f "delims=" %%i in ('go env GOPATH') do set "GARBLE=%%i\bin\garble.exe"
if not exist "%GARBLE%" (
  echo [x] 未找到 garble: %GARBLE%
  echo     请先安装: set GOTOOLCHAIN=local ^&^& go install mvdan.cc/garble@v0.15.0
  exit /b 1
)

echo [1/2] 混淆编译中 ^(garble -literals -tiny^) ...
"%GARBLE%" -literals -tiny build -trimpath -ldflags "-s -w" -o bailan-relay.exe .
if errorlevel 1 (echo [x] 混淆构建失败。& exit /b 1)

where upx >nul 2>nul && (echo [2/2] UPX 压缩中... & upx --best --lzma bailan-relay.exe) || echo [2/2] 跳过 UPX ^(未安装, 可选^)。

echo.
echo 混淆打包完成: %~dp0bailan-relay.exe
echo 运行时请以管理员身份启动。
