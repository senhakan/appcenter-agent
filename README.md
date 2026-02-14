# AppCenter Agent

Windows istemcilerde uygulama kurulumunu merkezi AppCenter Server komutlariyla yoneten Go tabanli agent.

## Durum

- Faz 1 tamamlandi (bootstrap, register, heartbeat, config, uuid).
- Faz 2 tamamlandi (downloader/resume, installer, task status client).
- Faz 3 tamamlandi (task queue, retry, UTC work-hours, jitter, apps_changed entegrasyonu).
- Faz 4 tamamlandi (Windows service wrapper + service install/build scriptleri).
- Faz 5 tamamlandi (Named Pipe IPC: get_status/get_store + tray client).
- Faz 6 tamamlandi (Windows systray UI: durum, store refresh, install istegi).
- GitHub Actions ile her push'ta test + Windows build calisiyor.

## Bu Repoda Olanlar

- Service giris noktasi: `cmd/service/main.go`
- Tray giris noktasi: `cmd/tray/main.go`
- API istemcisi: `internal/api/client.go`
- Config yonetimi: `internal/config/config.go`
- Heartbeat dongusu: `internal/heartbeat/heartbeat.go`
- Task queue + retry + work-hours: `internal/queue/taskqueue.go`
- Windows service wrapper: `cmd/service/main_windows.go`, `cmd/service/service_windows.go`
- Named Pipe IPC: `internal/ipc/*`
- Tray UI: `internal/tray/*`
- Downloader (rate limit + resume): `internal/downloader/downloader.go`
- Installer (`.msi` / `.exe`): `internal/installer/*`
- UUID + host info: `internal/system/*`
- Utility fonksiyonlar: `pkg/utils/*`

## Hemen Basla

```bash
# bagimliliklar
go mod tidy

# testler
go test ./...

# windows build (linux/macos ortaminda cross-compile)
GOOS=windows GOARCH=amd64 go build -o build/appcenter-service.exe ./cmd/service
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" -o build/appcenter-tray.exe ./cmd/tray

# windows'ta yardimci scriptler
build\\build.bat
build\\service-install.bat
```

## Konfigurasyon

Varsayilan config ornegi: `configs/config.yaml.template`

Runtime'da config yolu:

```bash
APPCENTER_CONFIG=/path/to/config.yaml ./appcenter-service
```

Gerekli minimum alanlar:

- `server.url`
- `agent.version`
- `heartbeat.interval_sec`
- `download.bandwidth_limit_kbps`

## CI/CD

Workflow: `.github/workflows/build.yml`

Pipeline adimlari:

- `go mod tidy`
- `go test ./...`
- Windows service build
- Windows tray build
- Artifact upload (`appcenter-agent-windows`)

## IPC Notu

- `get_status` ve `get_store` aksiyonlari aktif.
- `install_from_store` aksiyonu su an server-side deployment akisina yonlendirme gerektirdigi icin bilgilendirici hata doner.

## Tray Kullanimi

- Windows GUI mode:
  - `appcenter-tray.exe` (argumansiz) systray acilir.
- CLI mode (debug):
  - `appcenter-tray.exe get_status`
  - `appcenter-tray.exe get_store`
  - `appcenter-tray.exe install_from_store 12`

## Dokumanlar

- Teknik spesifikasyon: `AppCenter_Technical_Specification_v1_1.md`
- Agent gelistirme rehberi: `CLAUDE.md`
- Islem gecmisi: `docs/WORKLOG.md`
- MSI test adimlari: `docs/WINDOWS_MSI_TEST_GUIDE.md`

## Sonraki Asama

Faz 7:

- Self-update mekanizmasi
- Error handling ve log rotation iyilestirmeleri
