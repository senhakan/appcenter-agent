# Continuation (Handoff) Notes

Bu dokuman, bu repoda su ana kadar yapilan islerin uzerine sorunsuz devam edebilmek icin pratik bir "nerede kaldik" notudur.

## Mevcut Durum (Ozet)

- Agent Windows'ta service olarak calisir: `AppCenterAgent`
- Deployment akisi (Server -> Agent):
  - group bazli deployment test edildi (MSI kurulum, exit code raporlama, log dogrulama)
- Tray:
  - `appcenter-tray.exe` (windowsgui) ile systray icon gorunur
  - `appcenter-tray-cli.exe` (console) ile `get_status/get_store` ciktilari PowerShell'de gorunur
  - Tray autostart: MSI icinde HKLM Run kaydi ile all-users logon'da baslar
  - Duplicate instance: mutex ile engellenir
- Self-update:
  - Update staging yapilir (`pending_update.json`)
  - `update.auto_apply=true` ise `appcenter-update-helper.exe` ile staged update apply + service restart yapilir
- MSI exit code uyumlulugu:
  - `3010` (reboot required) ve `1641` (reboot initiated) agent tarafinda basari sayilir

Detay kronoloji: `docs/WORKLOG.md`

## MSI Nereden Alinacak? (CI Artifact)

MSI GitHub Actions ile uretilir:

- Workflow: `.github/workflows/build.yml`
- Job: `build-windows-msi`
- Artifact: `appcenter-agent-msi`

Bu ortamdan artifact indirmek icin bir yetkilendirme mekanizmasi gerekir (PAT veya Release publish gibi). Iki pratik opsiyon:

1. MSI artifact'ini GitHub UI'dan indirip Windows test makinesine koymak.
2. Workflow'u MSI'yi bir GitHub Release'e ekleyecek sekilde guncelleyip anonim indirilebilir yapmak.

## Windows Uzerinde MSI Test Akisi (Temiz VM Onerilir)

MSI; servis kurar/baslatir ve tray autostart kaydi yazar. Daha once script ile kurulmus bir ortamda "service already exists" gibi cakismalar olabilir; bu nedenle temiz snapshot/VM tercih edin.

Kontrol listesi:

1. Kurulum:
   - `msiexec /i AppCenterAgent-<version>.msi /qn /norestart`
2. Service dogrulama:
   - `Get-Service AppCenterAgent`
3. Dosyalar:
   - `C:\Program Files\AppCenter\appcenter-service.exe`
   - `C:\Program Files\AppCenter\appcenter-tray.exe`
   - `C:\Program Files\AppCenter\appcenter-tray-cli.exe`
   - `C:\Program Files\AppCenter\appcenter-update-helper.exe`
4. ProgramData:
   - `C:\ProgramData\AppCenter\config.yaml` (upgrade/repair overwrite etmemeli)
   - `C:\ProgramData\AppCenter\downloads`
   - `C:\ProgramData\AppCenter\logs`
5. Tray autostart:
   - `HKLM:\Software\Microsoft\Windows\CurrentVersion\Run` altinda `AppCenterTray`
6. Uctan-uca deployment smoke:
   - Server'da deployment olustur, agent'in indirip kurdugunu ve task status'in `success` oldugunu dogrula
7. Staged update apply smoke:
   - Server'da agent update upload et, heartbeat ile stage ettir, `auto_apply` ile helper'in replace+restart yaptigini dogrula

MSI odakli ayrinti: `docs/AGENT_INSTALLER_MSI.md`

