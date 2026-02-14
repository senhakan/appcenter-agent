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

## Kural

Bu dosya her teknik degisiklikten sonra guncellenir:

- Ne degisti?
- Hangi dosyalarda?
- Hangi test/build calisti?
- Sonuc ne oldu?
