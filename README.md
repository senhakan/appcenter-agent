# AppCenter Agent

Windows istemcilerde uygulama kurulumunu merkezi AppCenter Server komutlariyla yoneten Go tabanli agent.

## Durum

- Faz 1 tamamlandi (bootstrap, register, heartbeat, config, uuid).
- Faz 2 tamamlandi (downloader/resume, installer, task status client).
- Faz 3 tamamlandi (task queue, retry, UTC work-hours, jitter, apps_changed entegrasyonu).
- Faz 4 tamamlandi (Windows service wrapper + service install/build scriptleri).
- Faz 5 tamamlandi (Named Pipe IPC: get_status/get_store + tray client).
- Faz 6 tamamlandi (Windows systray UI: durum, store refresh, install istegi).
- Faz 7 tamamlandi (self-update staging, log rotation, error handling polish).
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
- Updater: `internal/updater/*`
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
GOOS=windows GOARCH=amd64 go build -o build/appcenter-tray-cli.exe ./cmd/tray

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
- `install_from_store` aksiyonu bu surumde bilerek kapali (server-side deployment akisina baglanacak).

## Self-Update Notu

- Heartbeat `config` alaninda gelen `latest_agent_version`, `agent_download_url`, `agent_hash` degerlerine gore update paketi indirip dogrular.
- Dogrulanan paket `download.temp_dir` altinda staged edilir:
  - `agent-update-<version>.exe`
  - `pending_update.json`
- Bu surumde update apply islemi otomatik restart/replace yapmaz; guvenli staging yapar.

## Tray Kullanimi

- Windows GUI mode:
  - `appcenter-tray.exe` (argumansiz) systray acilir.
- CLI mode (debug, console cikti icin):
  - `appcenter-tray-cli.exe get_status`
  - `appcenter-tray-cli.exe get_store`
  - `appcenter-tray-cli.exe install_from_store 12`

## Dokumanlar

- Teknik spesifikasyon: `AppCenter_Technical_Specification_v1_1.md`
- Agent gelistirme rehberi: `CLAUDE.md`
- Islem gecmisi: `docs/WORKLOG.md`
- MSI test adimlari: `docs/WINDOWS_MSI_TEST_GUIDE.md`

## Sonraki Asama

- `install_from_store` aksiyonunu server-side deployment akisiyla tamamlamak
- Staged update icin kontrollu apply/restart adimi eklemek
