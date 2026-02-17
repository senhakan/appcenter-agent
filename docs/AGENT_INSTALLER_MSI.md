# Agent Installer MSI

Bu dokuman, AppCenter Agent'in Windows client'lara MSI ile dagitimi icin uretilen installer'i aciklar.

## MSI Neleri Kurar?

- Paket tipi: `x64` (per-machine)
- Binary'ler:
  - `C:\Program Files\AppCenter\appcenter-service.exe`
  - `C:\Program Files\AppCenter\appcenter-tray.exe`
  - `C:\Program Files\AppCenter\appcenter-tray-cli.exe`
  - `C:\Program Files\AppCenter\appcenter-update-helper.exe`
- Windows Service:
  - Service name: `AppCenterAgent`
  - Start type: Automatic
- ProgramData config:
  - `C:\ProgramData\AppCenter\config.yaml`
  - `NeverOverwrite=yes`: upgrade/repair sirasinda yerel config degisiklikleri ezilmez.
- Runtime klasorleri:
  - `C:\ProgramData\AppCenter\downloads`
  - `C:\ProgramData\AppCenter\logs`
- Tray autostart (all users):
  - `HKLM\Software\Microsoft\Windows\CurrentVersion\Run\AppCenterTray`
- Tek dosya dagitim:
  - WiX `MediaTemplate EmbedCab="yes"` ile MSI icine cabinet gomulur.

## MSI Build (CI)

Workflow: `.github/workflows/build.yml`

- Job: `build-windows-msi`
- Artifact: `appcenter-agent-msi`

## MSI Build (Local / Windows)

Gereksinimler:

- Go 1.21+
- WiX Toolset v3.x (`candle.exe`, `light.exe`)

Komut:

```powershell
.\build\build-msi.ps1 -BuildDir build -SourceDir .
```

## Kurulum / Kaldirma

Silent install:

```powershell
msiexec /i .\AppCenterAgent-<version>.msi /qn /norestart
```

Silent uninstall:

```powershell
msiexec /x .\AppCenterAgent-<version>.msi /qn /norestart
```

## Dogrulama

- Service:
  - `Get-Service AppCenterAgent`
- Tray autostart:
  - Registry key: `HKLM:\Software\Microsoft\Windows\CurrentVersion\Run` (`AppCenterTray`)
- Config:
  - `C:\ProgramData\AppCenter\config.yaml`

## Notlar

- MSI, varsayilan olarak `configs/config.yaml.template` dosyasini `C:\ProgramData\AppCenter\config.yaml` olarak yerlestirir.
- Bu varsayilan dosyada `server.url` degeri test/template amacli olabilir. Service'in kurulumdan sonra otomatik calismasi icin ortamda erisilebilir gercek server adresi ile guncellenmelidir.
