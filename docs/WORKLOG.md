# Worklog

Bu dosya agent reposunda yapilan teknik adimlari kronolojik olarak kaydeder.

## 2026-02-14

### Altyapi kurulumu

- Agent repo iskeleti olusturuldu:
  - `cmd/`, `internal/`, `pkg/`, `configs/`, `.github/workflows/`
- Go module baslatildi: `go.mod`
- GitHub Actions build pipeline eklendi: `.github/workflows/build.yml`

### Faz 1 (Tamamlandi)

- Config yukleme/yazma:
  - `internal/config/config.go`
- UUID yonetimi:
  - Windows registry implementasyonu: `internal/system/uuid_windows.go`
  - Non-Windows fallback: `internal/system/uuid_nonwindows.go`
- Host bilgi toplama:
  - `internal/system/info.go`
- API istemci:
  - register + heartbeat: `internal/api/client.go`
- Heartbeat sender:
  - `internal/heartbeat/heartbeat.go`
- Service bootstrap:
  - `cmd/service/main.go`
- Utility:
  - logger: `pkg/utils/logger.go`
  - hash verify: `pkg/utils/hash.go`

### Faz 2 (Tamamlandi)

- Downloader:
  - Rate limit + resume + header auth
  - `internal/downloader/downloader.go`
- Installer:
  - orchestrator: `internal/installer/installer.go`
  - exe installer: `internal/installer/exe.go`
  - msi installer (windows): `internal/installer/msi_windows.go`
  - non-windows fallback: `internal/installer/msi_nonwindows.go`
- Task status report client:
  - `internal/api/client.go` (`ReportTaskStatus`)

### Testler

- API client unit test: `internal/api/client_test.go`
- Downloader resume test: `internal/downloader/downloader_test.go`
- Installer testleri: `internal/installer/installer_test.go`
- Yerel sonuc: `go test ./...` basarili
- Yerel cross-build:
  - `GOOS=windows GOARCH=amd64 go build ...` basarili

### CI Sonuclari

- Build Run #1 (Faz 1):
  - Commit: `ab4c9629afef6d79f00f050ce16c116a310ad4c3`
  - Run: `https://github.com/senhakan/appcenter-agent/actions/runs/22022087310`
  - Sonuc: success
- Build Run #2 (Faz 2):
  - Commit: `61ebcb847b82ddd9ffd509791f0d0755532dab25`
  - Run: `https://github.com/senhakan/appcenter-agent/actions/runs/22022157039`
  - Sonuc: success

### Remote Windows MSI Test (SSH ile)

- Test host: Windows test VM (IP/kullanici bilgileri repo disi tutulur)
- Erişim: Windows OpenSSH Server (PowerShell default shell, admin token gerekli)
- Hazırlık:
  - `C:\ProgramData\AppCenter\downloads`
  - `C:\ProgramData\AppCenter\logs`
  - Test paketi indirildi: `7zip-x64.msi`

Senaryo sonuçları:

- A (silent install):
  - Komut: `msiexec /i ... /qn /norestart`
  - Sonuç: `A_EXIT=0`
  - Doğrulama: `C:\Progra~1\7-Zip\7zFM.exe` bulundu
- B (eksik MSI dosyası):
  - Komut: `msiexec /i ...NOT_FOUND.msi /qn /norestart`
  - Sonuç: `B_EXIT=1619`
- C (geçersiz MSI paketi):
  - Komut: `msiexec /i invalid.msi /qn /norestart`
  - Sonuç: `C_EXIT=1620`

Not:

- `/badarg` senaryosu SSH/non-interactive oturumda MSI UI beklemesine takılabildiği için bu turda otomasyon dışı bırakıldı.
- Log dosyaları üretildi:
  - `C:\ProgramData\AppCenter\logs\msi-install.log`
  - `C:\ProgramData\AppCenter\logs\msi-missing.out`
  - `C:\ProgramData\AppCenter\logs\msi-invalid.out`

### Uctan-Uca Deployment Testi (Server -> Agent)

- Server'in Windows makinelerden erisilebilir olmasi icin test ortaminda `--host 0.0.0.0 --port 8000` ile dinleme acildi.
- Gercek deployment akisi test edildi:
  - Ilk denemede MSI `1641` (reboot initiated) kodu "failed" sayildi.
  - Fix: `3010` ve `1641` agent tarafinda basari olarak kabul edildi.
- Not: `exit_code=0` JSON omitempty nedeniyle server DB'ye `NULL` dusuyordu; exit_code alanini pointer ile gondererek `0` kaydinin korunmasi saglandi.

### Tray CLI Cikti Sorunu (Windows)

- `appcenter-tray.exe` `-H=windowsgui` ile derlendiginden PowerShell'de `get_status/get_store` komutlari stdout'a yazsa bile gorunmez.
- Cozum: ayni kod tabanindan ayrica console subsystem ile `appcenter-tray-cli.exe` artifact'i uretilir; CLI testleri bunda calistirilir.

### Group Bazli Deployment Testi (grp2 -> 9zip/7zip 26)

- Group olusturuldu: `grp2`
- Test agent bu gruba eklendi.
- Group hedefli deployment olusturuldu:
  - App: `9zip` (server tarafinda `original_filename=7z2600-x64.msi`)
  - Target: `grp2`
  - `force_update=true`
- Sonuc (server DB):
  - task `status=success`, `exit_code=0`
- Sonuc (Windows dogrulama):
  - `C:\\Program Files\\7-Zip\\7zFM.exe` dosya versiyonu: `26.00`
  - Windows Installer event log: `MsiInstaller` event'lerinde `7-Zip 26.00 (x64 edition) -- Installation completed successfully` kaydi goruldu.

### Staged Update Apply (Windows)

- `update.auto_apply=true` oldugunda service, staged edilen `pending_update.json` dosyasini gorunce update apply'i tetikler.
- Apply islemi service icinden degil, ayrik bir helper ile yapilir:
  - `appcenter-update-helper.exe` service'i durdurur
  - `C:\\Program Files\\AppCenter\\appcenter-service.exe` dosyasini backup alir
  - staged exe'yi hedef exe ile degistirir
  - config'te `agent.version` degerini hedef versiyona gunceller
  - service'i tekrar baslatir
- Not: helper process'inin service kapaninca olmeyecek sekilde detached baslatilmasi gerekir (context'e baglanmaz).

### System Tray Icon Testi (Windows)

- Tray icon sadece kullanici oturumunda gorunur; Windows service (Session 0) tray icon gosteremez.
- Ikon gorunmeme durumu icin not:
  - Windows'ta systray ikonunun gorunmesi icin `systray.SetIcon()` cagrisinin yapilmasi gerekir; aksi halde proses calissa bile ikon cikmayabilir.
- Test icin tray binary interaktif session'da calistirildi (RDP session):
  - `appcenter-tray-cli.exe` argumansiz calistirilinca systray mode acilir.
  - Gerekirse `schtasks /IT` ile interaktif session'a start edilebilir.

### Tray Autostart (All Users)

- Tray uygulamasi service degildir; kullanici oturumunda baslatilmalidir.
- Uretim dagitiminda bu is MSI icinde yapilir:
  - `HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Run` altina `AppCenterTray` kaydi eklenir.
  - Bu sayede her kullanici logon oldugunda tray otomatik calisir.
- Duplicate instance engelleme:
  - Windows'ta `Local\\AppCenterTray` mutex ile ayni session icinde ikinci instance engellenir.

### Faz 3 (Tamamlandi)

- Queue/retry/work-hours/jitter:
  - `internal/queue/taskqueue.go`
  - `internal/queue/taskqueue_test.go`
- Heartbeat queue entegrasyonu:
  - `internal/heartbeat/heartbeat.go`
  - `apps_changed` + `installed_apps` payload üretimi queue'dan besleniyor
- Service orchestration:
  - `cmd/service/main.go`
  - heartbeat komutlarını queue'ya alma, task yürütme, status report gönderme
- Executor akışı:
  - `internal/downloader/downloader.go` (`DownloadFileWithMeta`)
  - hash doğrulama + installer çağrısı + auto cleanup
- Config güncellemesi:
  - `internal/config/config.go`
  - `configs/config.yaml.template`
  - yeni alanlar: `install.timeout_sec`, `install.enable_auto_cleanup`

Doğrulama:

- `go test ./...` başarılı
- `GOOS=windows GOARCH=amd64 go build ...` başarılı

### Faz 4 (Tamamlandi)

- Windows service wrapper:
  - `cmd/service/main_windows.go`
  - `cmd/service/service_windows.go`
- Ortak runtime ayrıştırması:
  - `cmd/service/core.go`
  - `cmd/service/main.go` (`!windows` console entrypoint)
- Windows yardımcı scriptleri:
  - `build/build.bat`
  - `build/service-install.bat`

Doğrulama:

- `go test ./...` başarılı
- `GOOS=windows GOARCH=amd64 go build ...` başarılı

### Faz 5 (Tamamlandi)

- Named Pipe IPC:
  - `internal/ipc/namedpipe.go` (tipler)
  - `internal/ipc/namedpipe_windows.go` (server/client)
  - `internal/ipc/namedpipe_nonwindows.go` (fallback)
- Service IPC handler entegrasyonu:
  - `cmd/service/core.go`
  - aksiyonlar: `get_status`, `get_store`, `install_from_store`
- Tray IPC client:
  - `cmd/tray/main.go` (CLI tabanlı IPC çağrısı)
- API store client:
  - `internal/api/client.go` (`GetStore`)
  - `internal/api/client_test.go`
- IPC testleri:
  - `internal/ipc/namedpipe_test.go`
  - `internal/ipc/namedpipe_nonwindows_test.go`

Doğrulama:

- `go test ./...` başarılı
- `GOOS=windows GOARCH=amd64 go build ...` başarılı

### Faz 6 (Tamamlandi)

- Systray UI:
  - `internal/tray/systray_windows.go`
  - `internal/tray/systray_nonwindows.go`
  - `internal/tray/tray.go`
- Tray entrypoint:
  - `cmd/tray/main.go`
  - argümansız mod: systray
  - argümanlı mod: `get_status`, `get_store`, `install_from_store <app_id>`
- Store/status entegrasyonu:
  - service IPC endpointlerinden canlı çekim
  - systray tooltip/title güncellemesi
- Test:
  - `internal/tray/tray_test.go`

Doğrulama:

- `go test ./...` başarılı
- `GOOS=windows GOARCH=amd64 go build ...` başarılı

### Faz 7 (Tamamlandi)

- Self-update staging:
  - `internal/updater/updater.go`

### MSI Installer (WiX) (Tamamlandi)

- Hedef: service + tray autostart + ProgramData config/yol kurallari tek bir MSI ile dagitilabilsin.
- WiX tanimi:
  - `installer/wix/AppCenterAgent.wxs`
  - Kurulum yollari:
    - `C:\Program Files\AppCenter\*`
    - `C:\ProgramData\AppCenter\config.yaml` (NeverOverwrite)
  - Service: `AppCenterAgent` (Automatic, install + start)
  - Tray autostart (all users): HKLM Run `AppCenterTray`
- MSI build script:
  - `build/build-msi.ps1` (WiX v3 `candle.exe` + `light.exe`)
- CI job:
  - `.github/workflows/build.yml` -> `build-windows-msi`
  - Artifact: `appcenter-agent-msi`

Not:

- MSI artifact'ini bu ortamdan otomatik cekmek icin bir yetkilendirme/publish mekanizmasi gerekir (PAT veya Release).
  - Bu repo dokumanlari bilerek herhangi bir IP/kullanici/sifre bilgisi tutmaz.

## 2026-02-17

### MSI CI Duzeltmesi

- `build-windows-msi` job log incelemesinde iki kritik sorun tespit edildi:
  - `installer/wix/AppCenterAgent.wxs`: `NeverOverwrite` attribute'u `File` elementinde oldugu icin `candle.exe` hata veriyordu.
  - `build/build-msi.ps1`: `candle.exe` / `light.exe` hata kodlarini kontrol etmedigi icin script "MSI written" yazip basarili gorunebiliyordu.
- Yapilan fix:
  - `NeverOverwrite="yes"` `Component` seviyesine alindi (`cmpConfigFile`).
  - `build/build-msi.ps1` icine strict exit-code kontrolu eklendi:
    - `candle.exe` hata verirse throw
    - `light.exe` hata verirse throw
    - cikti MSI dosyasi yoksa throw
  - `installer/wix/AppCenterAgent.wxs` icin `MediaTemplate EmbedCab=\"yes\"` acildi:
    - MSI artifact tek dosya olarak dagitilabilir (ayri `.cab` bagimliligi olmaz).
  - MSI bitness duzeltmesi:
    - `Package Platform=\"x64\"`
    - `ProgramFiles64Folder`
    - Component'lerde `Win64=\"yes\"`
    - Neden: 32-bit MSI oldugunda dosyalar `Program Files (x86)` altina ve `AppCenterTray` kaydi `WOW6432Node` altina yaziliyordu.
- Beklenen sonuc:
  - Gercek hata durumunda CI fail olur (false-positive yok)
  - Basarili run'da `build/*.msi` artifact olarak yuklenir.
 - GitHub Actions test sonucu:
   - `appcenter-agent-msi` artifact'i olusuyor ve boyut beklenen seviyede (~13MB).
   - PAT ile artifact indirme dogrulandi.

### MSI Remote Smoke (Windows SSH)

- Test makinesine CI artifact MSI kopyalandi ve silent install calistirildi:
  - `msiexec /i ... /qn /norestart`
  - sonuc: `MSI_EXIT=0`
- Service ve dosya dogrulama:
  - `AppCenterAgent` service running
  - `C:\Program Files\AppCenter\appcenter-service.exe` mevcut
  - `C:\ProgramData\AppCenter\config.yaml` mevcut
- Onemli bulgu:
  - Onceki 32-bit MSI testinden kalan kayit/dosyalar nedeniyle ayni makinede hem `RUN64` hem `RUN32` gorulebildi.
  - Guncel x64 MSI icin hedef dogru anahtar:
    - `HKLM\Software\Microsoft\Windows\CurrentVersion\Run\AppCenterTray`

### MSI Temiz Sistem Simulasyonu (Ayni Test Makinesinde)

- Test makinesi temizlendi:
  - `AppCenterAgent` service stop/delete
  - Eski MSI urun kayitlari uninstall (`x86` + `x64` onceki urun kodlari)
  - `HKLM ... Run\AppCenterTray` hem 64-bit hem WOW6432Node temizligi
  - `C:\Program Files\AppCenter`, `C:\Program Files (x86)\AppCenter`, `C:\ProgramData\AppCenter` temizligi
- CI artifact ile tekrar test edildi (`appcenter-agent-msi`):
  - Install: `exit=0`
  - Uninstall: `exit=0`
  - Reinstall: `exit=0`
- Dogrulama (temiz dongu sonrasi):
  - Binary path: `C:\Program Files\AppCenter\appcenter-service.exe`
  - `PF32` kurulumu yok
  - Run key 64-bit hive'da, WOW6432Node bos
  - `config.yaml` olusuyor
- Ek bulgu:
  - Varsayilan `config.yaml` `server.url=http://127.0.0.1:8000` ile gelirse service bootstrap sirasinda cikabilir (`WIN32_EXIT_CODE=1`).
  - `server.url` test ortami server adresine guncellenince service `Running` oldu ve:
    - `appcenter-tray-cli.exe get_status` = `ok`
    - `appcenter-tray-cli.exe get_store` = `ok`

### MSI Parametreleri + Wizard (Tamamlandi)

- Hedef:
  - Kurulumda `server.url` ve `secret_key` degerleri MSI parametresi olarak verilebilsin.
  - Ayni degerler etkilesimli wizard ile de alinabilsin.
- Uygulama:
  - WiX public properties:
    - `SERVER_URL`
    - `SECRET_KEY` (Hidden)
  - MSI bu degerleri registry'ye yazar:
    - `HKLM\Software\AppCenter\Agent\Bootstrap\ServerURL`
    - `HKLM\Software\AppCenter\Agent\Bootstrap\SecretKey`
  - Agent config load sirasinda runtime override uygular:
    - Env: `APPCENTER_SERVER_URL`, `APPCENTER_SECRET_KEY`
    - Windows registry bootstrap key'leri
- Wizard:
  - `build/install-wizard.ps1`
  - URL ve secret alir, MSI'i parametreli calistirir.
  - Opsiyonel non-interactive parametreler:
    - `-ServerUrl`
    - `-SecretKey`
    - `-Silent`
- Remote test sonucu:
  - `msiexec /i ... SERVER_URL=... SECRET_KEY=... /qn` ile kurulum `exit=0`
  - Registry dogrulama:
    - `HKLM\Software\AppCenter\Agent\Bootstrap\ServerURL` yazildi
    - `HKLM\Software\AppCenter\Agent\Bootstrap\SecretKey` yazildi
  - Service `Running`, tray CLI `get_status=ok`

## 2026-02-17

### Agent Inventory Modulu (Windows) - Entegrasyon ve Canli Test

- Agent tarafinda inventory kodu eklendi:
  - `internal/inventory/inventory.go`
  - `internal/inventory/scanner_windows.go`
  - `internal/inventory/scanner_nonwindows.go`
- Heartbeat payload'ina inventory hash eklendi:
  - `internal/api/client.go` -> `HeartbeatRequest.InventoryHash`
  - `internal/heartbeat/heartbeat.go` -> `InventoryHashProvider`, `InventorySyncRequired`
- Service loop inventory akisi:
  - `cmd/service/core.go`
  - startup'ta `ForceScan()`
  - server config'ten `inventory_scan_interval_min` alinmasi
  - `inventory_sync_required=true` durumunda `/api/v1/agent/inventory` submit

Canli Windows test (host: `10.6.20.172`):

- Inventory build'i ile service binary guncellendi.
- Service start sonrasi log dogrulama:
  - `inventory force scan: 147 items, hash=...`
- Ilk durumda secret uyumsuzlugu nedeniyle `401 Unauthorized` goruldu.
- Secret sifirlanip yeniden register sonrasi:
  - `agent registered: 54d2ad5c-5b66-477d-82da-e5a22ef6dc01`
  - `heartbeat ok: status=ok commands=0`
  - `inventory submitted: Inventory updated (installed=0 removed=0 updated=0)`
- Server API dogrulama:
  - `GET /api/v1/agents/54d2ad5c-5b66-477d-82da-e5a22ef6dc01/inventory`
  - `total=147` kayit goruldu.
- Degisim gecmisi test rehberi eklendi:
  - `docs/INVENTORY_TEST_GUIDE.md`

### Inventory: MSIX/Appx (Microsoft Store) Uygulamalari

- NanaZip gibi Store (MSIX/Appx) ile gelen uygulamalar `HKLM/HKCU ...\\Uninstall` altinda gorunmez.
- Inventory taramasina `Get-AppxPackage -AllUsers` ciktilari eklendi.
- Dogrulama:
  - Windows'ta `Get-AppxPackage -AllUsers | ? Name -match NanaZip`
  - Server inventory listesinde `NanaZip` gorunmeli.
  - heartbeat `config` alanindan update bilgisi alinir
  - update paketi indirilir + hash dogrulanir + `pending_update.json` yazilir
- Log rotation:
  - `pkg/utils/logger.go`
  - `logging.max_size_mb`, `logging.max_backups` ile boyut bazli rotate
- Error handling polish:
  - `cmd/service/core.go`
  - task status report icin retry (3 deneme, artan bekleme)
  - updater hatalari main loop'u durdurmaz, loglanir
- Config update:
  - `internal/config/config.go`
  - `configs/config.yaml.template`
  - yeni alanlar: `logging.max_size_mb`, `logging.max_backups`
- Test:
  - `internal/updater/updater_test.go`

Doğrulama:

- `go test ./...` başarılı
- `GOOS=windows GOARCH=amd64 go build ...` başarılı

### Session Reporting (Login Olan Kullanici + local/RDP)

- Heartbeat payload'ina login olan kullanici session listesi eklendi:
  - Field: `logged_in_sessions: [{ username, session_type, logon_id? }]`
  - `session_type`: `local` veya `rdp`
- Windows tespiti:
  - PowerShell CIM/WMI ile `Win32_LogonSession` + `Win32_LoggedOnUser` asosiasyonu kullanilir.
  - Lokalize output parse edilmez (quser vb. yok).
- Server tarafinda persist + UI:
  - Heartbeat ile gelen liste server'da agents tablosuna JSON olarak yazilir.
  - Agent detail ekraninda goruntulenir.
- Canli test:
  - Test host: `10.6.20.172`
  - Agent self-update ile `0.1.9`'a guncellendi.
  - Server API dogrulama:
    - `GET /api/v1/agents/<uuid>` -> `logged_in_sessions` dolu geldi.

### System Profile Reporting (OS + Donanim + Virtualization) + Sistem Gecmisi

- Statik bilgiler periyodik gonderim:
  - Heartbeat alanı: `system_profile`
  - Siklik: `system_profile.report_interval_min` (default: 720 dk)
- Windows toplama (best-effort):
  - OS: `Win32_OperatingSystem` (Caption/Version/Build/Architecture)
  - Bilgisayar: `Win32_ComputerSystem` (Manufacturer/Model/TotalPhysicalMemory)
  - CPU: `Win32_Processor` (Name/Cores/Logical)
  - Disk: `Win32_DiskDrive` + `Get-Disk` (Model/Size/BusType)
  - Virtualization: Manufacturer+Model heuristic
- Canli dogrulama:
  - Test host: `10.6.20.172`
  - Agent self-update ile `0.1.12`'ye guncellendi.
  - Server API:
    - `GET /api/v1/agents/<uuid>` -> `system_profile` dolu geldi.
    - `GET /api/v1/agents/<uuid>/system/history` -> `total>=1`, ilk kayit `changed_fields=['initial']`.

## Kural

Bu dosya her teknik degisiklikten sonra guncellenir:

- Ne degisti?
- Hangi dosyalarda?
- Hangi test/build calisti?
- Sonuc ne oldu?
