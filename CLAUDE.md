# AppCenter Agent - Claude Code Geliştirme Rehberi

## PROJE HAKKINDA

AppCenter Agent, merkez sunucudan komut alarak Windows bilgisayarlara uygulama kuran bir istemci yazılımıdır.
Bu dosya **AGENT** tarafını kapsar. Server tarafı ayrı bir repository/session'da geliştirilir.

**Teknoloji:** Go 1.21+, Windows Service, System Tray, Named Pipes  
**Hedef Platform:** Windows 10/11, Server 2016+  
**Referans Doküman:** `../AppCenter_Technical_Specification_v1_1.md`

### UYGULANAN SON DURUM (2026-02-14)

- Faz 1 tamamlandı:
  - `go.mod`, `configs/config.yaml.template`
  - `internal/config/config.go`
  - `internal/system/info.go`, `internal/system/uuid_windows.go`, `internal/system/uuid_nonwindows.go`
  - `internal/api/client.go` (register + heartbeat)
  - `internal/heartbeat/heartbeat.go`
  - `cmd/service/main.go`
- Faz 2 tamamlandı:
  - `internal/downloader/downloader.go` (bandwidth limit + resume)
  - `internal/installer/installer.go`, `internal/installer/msi_windows.go`, `internal/installer/msi_nonwindows.go`, `internal/installer/exe.go`
  - `internal/api/client.go` (`ReportTaskStatus`)
- Faz 3 tamamlandı:
  - `internal/queue/taskqueue.go` (task queue + retry + UTC work-hours + jitter)
  - `internal/heartbeat/heartbeat.go` (`apps_changed` + queue integrated heartbeat payload)
  - `cmd/service/main.go` (heartbeat sonuçlarından queue besleme + task yürütme + status report)
- Faz 4 tamamlandı:
  - `cmd/service/main_windows.go` (`svc.IsWindowsService` ile mode tespiti)
  - `cmd/service/service_windows.go` (Windows service lifecycle: start/stop/shutdown)
  - `cmd/service/core.go` (service/console ortak runtime çekirdeği)
  - `build/build.bat`, `build/service-install.bat`
- Faz 5 tamamlandı:
  - `internal/ipc/namedpipe_windows.go`, `internal/ipc/namedpipe_nonwindows.go`, `internal/ipc/namedpipe.go`
  - `cmd/service/core.go` içinde IPC server + handler (`get_status`, `get_store`, `install_from_store`)
  - `cmd/tray/main.go` içinde IPC client (CLI tabanlı)
- Faz 6 tamamlandı:
  - `internal/tray/systray_windows.go`, `internal/tray/systray_nonwindows.go`, `internal/tray/tray.go`
  - `cmd/tray/main.go` argümansız modda systray UI, argümanlı modda IPC CLI
  - Systray menüsü: status refresh, store refresh, install_from_store tetikleme
- Faz 7 tamamlandı:
  - `internal/updater/updater.go` (heartbeat config ile self-update paketi staging)
  - `pkg/utils/logger.go` (boyut bazlı log rotation)
  - `cmd/service/core.go` (task status report retry + updater entegrasyonu)
- Test ve derleme:
  - `go test ./...` başarılı
  - `GOOS=windows GOARCH=amd64` cross-build başarılı
  - GitHub Actions build:
    - `ab4c962` -> success
    - `61ebcb8` -> success

---

## DİZİN YAPISI

```
agent/
├── cmd/
│   ├── service/
│   │   └── main.go                 # Windows Service entry point
│   └── tray/
│       └── main.go                 # System Tray entry point
├── internal/
│   ├── config/
│   │   └── config.go               # YAML config okuma/yazma
│   ├── api/
│   │   └── client.go               # HTTP client (register, heartbeat, download, report)
│   ├── heartbeat/
│   │   └── heartbeat.go            # Periyodik heartbeat gönderici
│   ├── installer/
│   │   ├── installer.go            # Install orchestrator
│   │   ├── msi_windows.go          # msiexec wrapper (Windows)
│   │   ├── msi_nonwindows.go       # non-Windows fallback
│   │   └── exe.go                  # EXE wrapper
│   ├── downloader/
│   │   └── downloader.go           # Bandwidth-limited downloader + resume
│   ├── system/
│   │   ├── info.go                 # Hostname, IP, OS, CPU, RAM bilgisi
│   │   ├── uuid_windows.go         # Registry'den UUID oku/yaz
│   │   └── uuid_nonwindows.go      # non-Windows fallback UUID
│   ├── queue/
│   │   └── taskqueue.go            # Task queue + retry (exponential backoff)
│   ├── tray/
│   │   └── tray.go                 # System tray placeholder
│   └── ipc/
│       └── namedpipe.go            # Named Pipe server (service) + client (tray)
├── pkg/
│   └── utils/
│       ├── hash.go                 # SHA256 doğrulama
│       └── logger.go               # Dosyaya loglama
├── configs/
│   └── config.yaml.template
├── build/
│   ├── build.bat                   # Derleme script'i
│   └── service-install.bat         # Kurulum script'i
├── go.mod
├── go.sum
└── README.md
```

---

## KRİTİK KURALLAR

### IPC: Named Pipes (dosya bazlı IPC YOK)
- Pipe adı: `\\.\pipe\AppCenterIPC`
- Service tarafı: pipe server olarak dinler
- Tray tarafı: pipe client olarak bağlanır
- Kütüphane: `github.com/Microsoft/go-winio`
- JSON mesajlaşma: `IPCRequest` → `IPCResponse`
- `requests.json` KULLANILMIYOR

### İki Ayrı Binary
- `appcenter-service.exe` → Windows Service (SYSTEM olarak çalışır)
- `appcenter-tray.exe` → System Tray (kullanıcı oturumunda çalışır, `-H=windowsgui` flag ile derlenir)

### Dosya Tipleri
- Sadece `.msi` ve `.exe` desteklenir (`.zip` YOK)

### API İletişimi
- Register: `POST /api/v1/agent/register`
- Heartbeat: `POST /api/v1/agent/heartbeat` (her 60 saniye)
  - Header: `X-Agent-UUID`, `X-Agent-Secret`
  - `apps_changed: true` sadece install/uninstall sonrası, normalde `false` gönder
- Download: `GET /api/v1/agent/download/{app_id}` (Range header desteği)
- Task rapor: `POST /api/v1/agent/task/{task_id}/status`
- Store: `GET /api/v1/agent/store` (UUID header'da, path'te DEĞİL)

### Zaman
- Tüm zaman damgaları UTC
- `server_time` heartbeat response'undan alınır
- Time window kontrolü UTC ile yapılır

### Registry
- `HKLM\SOFTWARE\AppCenter\UUID` - Agent kimliği
- `HKLM\SOFTWARE\AppCenter\SecretKey` - Windows DPAPI ile şifreli

### Kurulum Yolları
- Binary: `C:\Program Files\AppCenter\`
- Data: `C:\ProgramData\AppCenter\`
- Downloads: `C:\ProgramData\AppCenter\downloads\`
- Logs: `C:\ProgramData\AppCenter\logs\`
- Config: `C:\ProgramData\AppCenter\config.yaml`

---

## GELİŞTİRME SIRASI

### Faz 1: Temel Altyapı
1. `go.mod` - Dependency'ler tanımla
2. `internal/config/config.go` - YAML okuma/yazma
3. `internal/system/uuid_windows.go` (+ `uuid_nonwindows.go`) - UUID oku/oluştur
4. `internal/system/info.go` - Hostname, IP, OS version, CPU, RAM
5. `internal/api/client.go` - HTTP client (register, heartbeat)
6. `internal/heartbeat/heartbeat.go` - Timer ile periyodik gönderim
7. `cmd/service/main.go` - Basit main loop (henüz service wrapper olmadan test)

**Test:** `go run cmd/service/main.go` → Server'a register + heartbeat gönderebiliyor mu?

### Faz 2: Download & Install
1. `internal/downloader/downloader.go` - Bandwidth limit + resume
2. `pkg/utils/hash.go` - SHA256 doğrulama
3. `internal/system/disk.go` - Disk alan kontrolü
4. `internal/installer/msi_windows.go` - msiexec çağrısı
5. `internal/installer/exe.go` - EXE çağrısı
6. `internal/installer/installer.go` - Orchestrator (dosya tipine göre yönlendir)
7. `internal/api/client.go` - Task status raporlama ekle

**Test:** Heartbeat'ten gelen task'ı indir → hash doğrula → kur → rapor et

### Faz 3: Task Queue & Retry
1. [x] `internal/queue/taskqueue.go` - Queue + retry logic
2. [x] Time window checker (UTC)
3. [x] Jitter ekleme (0-5 dk rastgele gecikme)
4. [x] `apps_changed` flag mantığı

**Test:** Başarısız kurulum → retry → max 3 deneme → failed raporu

### Faz 4: Windows Service
1. [x] `cmd/service/main_windows.go` + `cmd/service/service_windows.go` - `golang.org/x/sys/windows/svc` entegrasyonu
2. [x] Service install/start/stop (`build/service-install.bat`)
3. [x] Graceful shutdown (service stop/shutdown sinyali ile context cancel)
4. [x] Log dosyasına yazma (ortak runtime `core.go` üzerinden)

**Test:** `sc create AppCenterAgent` → reboot → otomatik başlıyor mu?

### Faz 5: Named Pipe IPC
1. [x] `internal/ipc/namedpipe_windows.go` - Pipe server (service tarafı)
2. [x] `internal/ipc/namedpipe_windows.go` - Pipe client (tray tarafı)
3. [x] Request/Response tipleri: get_status, install_from_store, get_store

**Test:** Service çalışırken → Tray'den pipe'a bağlan → status al

### Faz 6: System Tray
1. [x] `internal/tray/systray_windows.go` - menu + status updater + store refresh
2. [x] `internal/tray/tray.go` - tray model/helper fonksiyonları
3. [x] `cmd/tray/main.go` - systray entrypoint + IPC CLI modu
4. [ ] Icon embed (go:embed) (sonraki iterasyon)

**Test:** Tray icon görünüyor mu? Store açılıyor mu? Install isteği service'e ulaşıyor mu?

### Faz 7: Self-Update & Polish
1. [x] Self-update staging (download + hash verify + metadata)
2. [x] Error handling iyileştirmesi (task status report retry)
3. [x] Log rotation (max_size_mb/max_backups)

---

## KOMUTLAR

```bash
# Go modülleri
cd agent
go mod init appcenter-agent
go mod tidy

# Derleme
# Service (normal console app olarak test)
go build -o build/appcenter-service.exe ./cmd/service/

# Service (production)
go build -ldflags="-s -w" -o build/appcenter-service.exe ./cmd/service/

# Tray (GUI - console penceresi açmaz)
go build -ldflags="-s -w -H=windowsgui" -o build/appcenter-tray.exe ./cmd/tray/

# Test
go test ./... -v

# Cross-compile (Linux'tan Windows'a)
GOOS=windows GOARCH=amd64 go build -o build/appcenter-service.exe ./cmd/service/
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" -o build/appcenter-tray.exe ./cmd/tray/
```

---

## BAĞIMLILIKLAR

```
github.com/getlantern/systray v1.2.2       # System tray
github.com/google/uuid v1.5.0              # UUID oluşturma
github.com/Microsoft/go-winio v0.6.1       # Named Pipes
golang.org/x/sys v0.16.0                   # Windows service + registry
golang.org/x/time v0.5.0                   # Rate limiting
gopkg.in/yaml.v3 v3.0.1                    # Config dosyası
```

---

## NOTLAR

- Server tarafı Python/FastAPI ile yazılıyor, bu repo'da DEĞİL
- API contract: `../AppCenter_Technical_Specification_v1_1.md` dosyasındaki endpoint tanımlarına uy
- Windows-specific API'ler (registry, service, DPAPI) için `golang.org/x/sys/windows` kullan
- Geliştirme sırasında service wrapper'sız test et (Faz 1-3), sonra service'e sar (Faz 4)
- Named Pipe'lar Windows-only, Linux'ta çalışmaz - cross-compile test'leri sadece build kontrolü için
