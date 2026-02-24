@echo off
setlocal

echo Building AppCenter Agent...

set ROOT=%~dp0..
pushd %ROOT%

if not exist build mkdir build

REM Service binary
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build\appcenter-service.exe .\cmd\service
if errorlevel 1 goto :fail

REM Tray binary
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o build\appcenter-tray.exe .\cmd\tray
if errorlevel 1 goto :fail

REM Tray CLI binary (console subsystem for stdout/stderr)
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build\appcenter-tray-cli.exe .\cmd\tray
if errorlevel 1 goto :fail

REM Update helper binary
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build\appcenter-update-helper.exe .\cmd\update-helper
if errorlevel 1 goto :fail

REM Remote support helper (custom build)
if exist ".\rshelper.exe" (
    copy /Y ".\rshelper.exe" "build\rshelper.exe" >nul
) else (
    powershell -NoProfile -ExecutionPolicy Bypass -File .\build\fetch-ultravnc-helper.ps1 -OutDir build
    if errorlevel 1 goto :fail
)

REM Native Store UI (C# WinForms)
dotnet publish .\ui\store-ui\AppCenter.StoreUI.csproj -c Release -r win-x64 --self-contained false -p:PublishSingleFile=true -o build
if errorlevel 1 goto :fail

copy /Y configs\config.yaml.template build\config.yaml >nul

echo Build completed.
popd
exit /b 0

:fail
echo Build failed.
popd
exit /b 1
