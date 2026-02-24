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
  - Server policy (`store_tray_enabled`) ile service tarafindan yonetilir:
    - `true` ise tray acik tutulur (kapanirsa yeniden acilir)
    - `false` ise tray kapatilir
- Self-update:
  - Update staging yapilir (`pending_update.json`)
  - `update.auto_apply=true` ise `appcenter-update-helper.exe` ile staged update apply + service restart yapilir
- Session reporting:
  - Heartbeat payload'ina login olan kullanicilar (local/RDP) eklendi: `logged_in_sessions`
  - Server agent detail ekraninda goruntulenir (server `logged_in_sessions_json` olarak persist eder)
- System profile reporting:
  - OS/donanim/virtualization profili periyodik olarak heartbeat ile gonderilir: `system_profile`
  - Siklik: `system_profile.report_interval_min` (default: 720 dk)
- MSI exit code uyumlulugu:
  - `3010` (reboot required) ve `1641` (reboot initiated) agent tarafinda basari sayilir

### Uzak Destek Web Goruntuleme (Guncel Durum)

- noVNC (aktif/varsayilan yol):
  - Session ekrani noVNC ile calisir.
  - `novnc-ticket` API + `/novnc-ws` WS bridge kullanilir.
  - `REMOTE_SUPPORT_NOVNC_MODE=embedded|iframe` ve `REMOTE_SUPPORT_WS_MODE=internal|external` kombinasyonlari desteklenir.
  - Uretimde onerilen profil: `embedded + internal`.

- Guacamole (pasif/fallback):
  - Varsayilan olarak devre disidir.
  - Gerekirse server tarafindaki `config/guacamole/REENABLE.md` adimlari ile tekrar alinabilir.

- Kayit ihtiyaci notu:
  - noVNC hattinda merkezi kayit/playback yerlesik degildir; bu ihtiyac icin ek cozum gereklidir.

### Denenip Birakilanlar (Kisa)

- Guacamole custom JS (`guacamole-common-js`) dogrudan embed:
  - Sahada siyah ekran/tutarsizlik nedeniyle birakildi.
- noVNC sadece CDN import:
  - Bazi ortamlarda script yukleme/tetikleme tutarsizligi goruldu, fallback ihtiyaci dogdu.
- noVNC `vnc_lite.html` custom baglanti:
  - Bagli gorunup goruntu gelmeyen senaryolar goruldu, `vnc.html` tabanli mode gecildi.

Detay kronoloji: `docs/WORKLOG.md`

### Store Grubu Kurgusu (2026-02-24)

- Server'da `Store` adli sistem grubu tanimlidir.
- Bu grup UI/API seviyesinde silinemez ve adi degistirilemez.
- Ajan bu gruptaysa heartbeat config'te `store_tray_enabled=true` alir.
- Ajan gruptan cikarildiginda `store_tray_enabled=false` olur ve tray kapatilir.

## Windows Test Ortami

- Host: `10.6.20.172`
- SSH kullanici: `apptest`
- SSH sifre: `1234asd!!!` (gecici/lab ortami)
- Baglanti ornegi:
  - `ssh apptest@10.6.20.172`
  - `scp <local-file> apptest@10.6.20.172:C:/Temp/`

## Inventory Test Notu

- Inventory/degisim gecmisi test adimlari icin:
  - `docs/INVENTORY_TEST_GUIDE.md`

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
