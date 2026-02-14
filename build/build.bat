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

copy /Y configs\config.yaml.template build\config.yaml >nul

echo Build completed.
popd
exit /b 0

:fail
echo Build failed.
popd
exit /b 1
