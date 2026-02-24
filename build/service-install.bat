@echo off
setlocal

net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ERROR: Administrator rights required.
    exit /b 1
)

set SRC=%~dp0
set TARGET_BIN=C:\Program Files\AppCenter
set TARGET_DATA=C:\ProgramData\AppCenter

mkdir "%TARGET_BIN%" 2>nul
mkdir "%TARGET_DATA%\downloads" 2>nul
mkdir "%TARGET_DATA%\logs" 2>nul

copy /Y "%SRC%appcenter-service.exe" "%TARGET_BIN%\appcenter-service.exe" >nul
copy /Y "%SRC%appcenter-tray.exe" "%TARGET_BIN%\appcenter-tray.exe" >nul
if exist "%SRC%appcenter-tray-cli.exe" (
    copy /Y "%SRC%appcenter-tray-cli.exe" "%TARGET_BIN%\appcenter-tray-cli.exe" >nul
)
if exist "%SRC%appcenter-update-helper.exe" (
    copy /Y "%SRC%appcenter-update-helper.exe" "%TARGET_BIN%\appcenter-update-helper.exe" >nul
)
if exist "%SRC%acremote-helper.exe" (
    copy /Y "%SRC%acremote-helper.exe" "%TARGET_BIN%\acremote-helper.exe" >nul
)
if exist "%SRC%appcenter-store-ui.exe" (
    copy /Y "%SRC%appcenter-store-ui.exe" "%TARGET_BIN%\appcenter-store-ui.exe" >nul
)
copy /Y "%SRC%config.yaml" "%TARGET_DATA%\config.yaml" >nul

sc query AppCenterAgent >nul 2>&1
if %errorLevel% equ 0 (
    sc stop AppCenterAgent >nul 2>&1
    sc delete AppCenterAgent >nul 2>&1
    timeout /t 2 >nul
)

sc create AppCenterAgent binPath= "\"%TARGET_BIN%\appcenter-service.exe\"" start= auto
if %errorLevel% neq 0 (
    echo ERROR: service create failed.
    exit /b 1
)

sc description AppCenterAgent "AppCenter Agent Service"
sc start AppCenterAgent

REM Start tray for every user on logon.
REM Note: the tray is optional UI; the service keeps working without it.
reg add "HKLM\Software\Microsoft\Windows\CurrentVersion\Run" /v AppCenterTray /t REG_SZ /d "\"%TARGET_BIN%\appcenter-tray.exe\"" /f >nul

echo Service installation completed.
exit /b 0
