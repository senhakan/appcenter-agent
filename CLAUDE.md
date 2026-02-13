# AppCenter Agent - Claude Code Geliştirme Rehberi

## PROJE HAKKINDA

AppCenter Agent, merkez sunucudan komut alarak Windows bilgisayarlara uygulama kuran bir istemci yazılımıdır.
Bu dosya **AGENT** tarafını kapsar. Server tarafı ayrı bir repository/session'da geliştirilir.

**Teknoloji:** Go 1.21+, Windows Service, System Tray, Named Pipes  
**Hedef Platform:** Windows 10/11, Server 2016+  
**Referans Doküman:** `../AppCenter_Technical_Specification_v1_1.md`

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
│   │   ├── msi.go                  # msiexec wrapper
│   │   └── exe.go                  # EXE wrapper
│   ├── downloader/
│   │   └── downloader.go           # Bandwidth-limited downloader + resume
│   ├── system/
│   │   ├── info.go                 # Hostname, IP, OS, CPU, RAM bilgisi
│   │   ├── uuid.go                 # Registry'den UUID oku/yaz
│   │   └── disk.go                 # Disk boş alan kontrolü
│   ├── queue/
│   │   └── taskqueue.go            # Task queue + retry (exponential backoff)
│   ├── tray/
│   │   ├── tray.go                 # System tray icon + menu
│   │   └── store.go                # Store penceresi
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
3. `internal/system/uuid.go` - Registry UUID oku/oluştur
4. `internal/system/info.go` - Hostname, IP, OS version, CPU, RAM
5. `internal/api/client.go` - HTTP client (register, heartbeat)
6. `internal/heartbeat/heartbeat.go` - Timer ile periyodik gönderim
7. `cmd/service/main.go` - Basit main loop (henüz service wrapper olmadan test)

**Test:** `go run cmd/service/main.go` → Server'a register + heartbeat gönderebiliyor mu?

### Faz 2: Download & Install
1. `internal/downloader/downloader.go` - Bandwidth limit + resume
2. `pkg/utils/hash.go` - SHA256 doğrulama
3. `internal/system/disk.go` - Disk alan kontrolü
4. `internal/installer/msi.go` - msiexec çağrısı
5. `internal/installer/exe.go` - EXE çağrısı
6. `internal/installer/installer.go` - Orchestrator (dosya tipine göre yönlendir)
7. `internal/api/client.go` - Task status raporlama ekle

**Test:** Heartbeat'ten gelen task'ı indir → hash doğrula → kur → rapor et

### Faz 3: Task Queue & Retry
1. `internal/queue/taskqueue.go` - Queue + retry logic
2. Time window checker (UTC)
3. Jitter ekleme (0-5 dk rastgele gecikme)
4. `apps_changed` flag mantığı

**Test:** Başarısız kurulum → retry → max 3 deneme → failed raporu

### Faz 4: Windows Service
1. `cmd/service/main.go` - `golang.org/x/sys/windows/svc` entegrasyonu
2. Service install/start/stop
3. Graceful shutdown
4. Log dosyasına yazma

**Test:** `sc create AppCenterAgent` → reboot → otomatik başlıyor mu?

### Faz 5: Named Pipe IPC
1. `internal/ipc/namedpipe.go` - Pipe server (service tarafı)
2. `internal/ipc/namedpipe.go` - Pipe client (tray tarafı)
3. Request/Response tipleri: get_status, install_from_store, get_store

**Test:** Service çalışırken → Tray'den pipe'a bağlan → status al

### Faz 6: System Tray
1. `internal/tray/tray.go` - Icon, menu, status updater
2. `internal/tray/store.go` - Store penceresi (basit liste + install butonu)
3. `cmd/tray/main.go` - Entry point
4. Icon embed (go:embed)

**Test:** Tray icon görünüyor mu? Store açılıyor mu? Install isteği service'e ulaşıyor mu?

### Faz 7: Self-Update & Polish
1. Self-update mechanism (download → rename → restart)
2. Error handling iyileştirmesi
3. Log rotation

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
