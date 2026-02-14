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

## Kural

Bu dosya her teknik degisiklikten sonra guncellenir:

- Ne degisti?
- Hangi dosyalarda?
- Hangi test/build calisti?
- Sonuc ne oldu?
