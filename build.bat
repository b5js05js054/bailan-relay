@echo off
REM 构建白岚 Devin 中继客户端（Go 版，无中间人 / hosts 劫持 + 原生 HTTPS）。
REM 说明：保留控制台窗口作为生命周期入口（关闭窗口即退出并恢复 hosts），
REM       因此不使用 -H windowsgui。
setlocal
cd /d "%~dp0"

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

echo [1/2] go mod tidy ...
go mod tidy
if errorlevel 1 goto :err

echo [2/2] go build ...
go build -trimpath -ldflags "-s -w" -o bailan-relay.exe .
if errorlevel 1 goto :err

echo.
echo 构建完成: %~dp0bailan-relay.exe
echo 运行时请以管理员身份启动（写 hosts / 装证书需要管理员权限）。
goto :eof

:err
echo 构建失败。
exit /b 1
