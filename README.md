# AppCenter Agent

Windows istemcilerde uygulama kurulumunu merkezi AppCenter Server komutlariyla yoneten Go tabanli agent.

## Durum

- Faz 1 tamamlandi (bootstrap, register, heartbeat, config, uuid).
- Faz 2 tamamlandi (downloader/resume, installer, task status client).
- Faz 3 tamamlandi (task queue, retry, UTC work-hours, jitter, apps_changed entegrasyonu).
- GitHub Actions ile her push'ta test + Windows build calisiyor.

## Bu Repoda Olanlar

- Service giris noktasi: `cmd/service/main.go`
- Tray giris noktasi (placeholder): `cmd/tray/main.go`
- API istemcisi: `internal/api/client.go`
- Config yonetimi: `internal/config/config.go`
- Heartbeat dongusu: `internal/heartbeat/heartbeat.go`
- Task queue + retry + work-hours: `internal/queue/taskqueue.go`
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

## Dokumanlar

- Teknik spesifikasyon: `AppCenter_Technical_Specification_v1_1.md`
- Agent gelistirme rehberi: `CLAUDE.md`
- Islem gecmisi: `docs/WORKLOG.md`
- MSI test adimlari: `docs/WINDOWS_MSI_TEST_GUIDE.md`

## Sonraki Asama

Faz 4:

- Gercek Windows service wrapper (`x/sys/windows/svc`)
- Service install/start/stop lifecycle
- Graceful shutdown iyilestirmeleri
