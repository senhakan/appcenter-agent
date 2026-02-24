# AppCenter Agent - Gelistirme Plani

**Son Guncelleme:** 2026-02-21
**Referans:** `../PLAN.md` (genel), `../REMOTE_SUPPORT_PLAN.md` (uzak destek detay)
**Dil:** Go 1.21+
**Hedef:** Windows 10/11, Server 2016+

---

## Mevcut Durum

### Tamamlanan Fazlar (1-7)
Agent uretim ortaminda calismakta:
- Windows Service (`AppCenterAgent`) + Tray (`appcenter-tray.exe`)
- Register, heartbeat (60s), task queue (retry, work-hours, jitter)
- Download (bandwidth limit + resume) + install (MSI/EXE)
- Named Pipe IPC (service ↔ tray)
- System tray UI (status, store, ikon degisimi)
- Self-update (staging + auto-apply + update-helper)
- Inventory modulu (yazilim tarama + hash bazli sync)
- Session reporting (login kullanicilari, local/RDP)
- System profile reporting (periyodik OS/donanim snapshot)
- MSI installer (WiX, parametreli, service + tray autostart)

### Kalan Isler (Mevcut)
- `install_from_store` aksiyonunu server-side deployment akisiyla tamamlamak
- MSI upgrade/uninstall testlerini temiz VM uzerinde dogrulamak
- Self-update failure rollback senaryolari

### Faz 8 Gerceklesen Durum Notu (2026-02-24)

- Agent tarafi kritik karar:
  - WinVNC/UltraVNC yasam dongusu test ortaminda kullanici yonetiminde birakildi.
  - Agent, onay/ready/session state raporlamasina odakli calisir; WinVNC prosesini zorla yonetmez.

- Server/web tarafi fiili cozum (guncel):
  - Uretimde aktif goruntuleme akisi noVNC'dir.
  - Session sayfasi `novnc-ticket` + `/novnc-ws` (internal WS bridge) ile calisir.
  - `embedded` ve `iframe` modlari desteklenir; baglanti oturum durumuna gore otomatik veya manuel baslatilir.

- Guacamole durumu:
  - Varsayilan olarak pasiftir.
  - Gerekirse server tarafinda `config/guacamole/REENABLE.md` adimlariyla tekrar devreye alinabilir.

- Denenip birakilan teknik yollar:
  - Guacamole `guacamole-common-js` ile dogrudan custom render.
  - noVNC `vnc_lite.html` custom RFB varyanti.
  - Sadece CDN import (fallbacksiz) noVNC yukleme.

- Kayit ihtiyaci notu:
  - noVNC hattinda merkezi kayit/playback yerlesik degildir; bu ihtiyac olursa ek katman planlanir.

---

## Faz 8: Uzak Destek - Agent Tarafindaki Isler

### 8.1 UltraVNC Helper Hazirlik (mevcut binary ile)

Guncel karar (2026-02-20): Ilk asamada fork/rebrand beklenmeyecek.
Mevcut `acremote-helper.exe` binary'si dogrudan kullanilarak entegrasyon tamamlanacak.
Rebrand/fork sonraki adimda ayri is paketi olarak yapilacak.

Cikti: Calisan bir `acremote-helper.exe` -> agent build/MSI'a dahil edilir.

### 8.3 Agent Degisiklikleri (2 gun)

#### 8.3.1 Yeni Paket: `internal/remotesupport/`

##### vnc.go - VNC Server Yasam Dongusu

```go
// internal/remotesupport/vnc.go
package remotesupport

import (
    "fmt"
    "log"
    "os"
    "os/exec"
    "path/filepath"
    "time"
)

const (
    vncHelperName    = "acremote-helper.exe"
    vncConfigName    = "acremote.ini"
    startupDelay     = 2 * time.Second
)

// VNCServer, UltraVNC SC fork binary'sini yonetir.
type VNCServer struct {
    exePath    string
    configPath string
    process    *os.Process
    logger     *log.Logger
}

// NewVNCServer, agent exe dizinindeki acremote-helper.exe'yi bulur.
// configPath: runtime ini dosyasinin yazilacagi yer (ProgramData dizini).
func NewVNCServer(logger *log.Logger) *VNCServer {
    exeDir := exeDirectory()
    return &VNCServer{
        exePath:    filepath.Join(exeDir, vncHelperName),
        configPath: filepath.Join(dataDirectory(), vncConfigName),
        logger:     logger,
    }
}

// Available, VNC helper binary'sinin mevcut olup olmadigini kontrol eder.
func (v *VNCServer) Available() bool {
    _, err := os.Stat(v.exePath)
    return err == nil
}

// Start, VNC server'i baslatir ve reverse connection kurar.
// password: one-time VNC sifresi (server tarafindan uretilir).
// guacdHost: reverse connection hedefi (sunucu adresi).
// guacdPort: reverse connection portu (varsayilan 5500).
func (v *VNCServer) Start(password, guacdHost string, guacdPort int) error {
    if !v.Available() {
        return fmt.Errorf("VNC helper not found: %s", v.exePath)
    }

    // Config dosyasini one-time password ile yaz
    if err := v.writeConfig(password); err != nil {
        return fmt.Errorf("write vnc config: %w", err)
    }

    // VNC server'i baslat
    cmd := exec.Command(v.exePath, "-run")
    cmd.Dir = filepath.Dir(v.exePath)
    if err := cmd.Start(); err != nil {
        return fmt.Errorf("start vnc server: %w", err)
    }
    v.process = cmd.Process
    v.logger.Printf("VNC server started (PID %d)", v.process.Pid)

    // Baslamasini bekle
    time.Sleep(startupDelay)

    // Reverse connection kur
    connectCmd := exec.Command(v.exePath, "-connect",
        fmt.Sprintf("%s::%d", guacdHost, guacdPort))
    if err := connectCmd.Run(); err != nil {
        v.logger.Printf("VNC reverse connect warning: %v", err)
        // Hata olsa bile devam et - baglanti daha sonra kurulabilir
    }

    v.logger.Printf("VNC reverse connection initiated to %s::%d", guacdHost, guacdPort)
    return nil
}

// Stop, VNC server'i durdurur ve config dosyasini temizler.
func (v *VNCServer) Stop() error {
    // -kill komutu ile duzenli kapatma
    killCmd := exec.Command(v.exePath, "-kill")
    if err := killCmd.Run(); err != nil {
        v.logger.Printf("VNC kill command: %v (trying process kill)", err)
        // Fallback: process'i dogrudan oldur
        if v.process != nil {
            _ = v.process.Kill()
        }
    }
    v.process = nil

    // Config dosyasindaki sifreyi temizle
    v.cleanConfig()

    v.logger.Printf("VNC server stopped and config cleaned")
    return nil
}

// IsRunning, VNC process'inin hala calisip calismadigini kontrol eder.
func (v *VNCServer) IsRunning() bool {
    if v.process == nil {
        return false
    }
    // Windows'ta process'in hala calisip calismadigini kontrol et
    // FindProcess her zaman basarili olur Windows'ta, Signal(0) ile kontrol et
    p, err := os.FindProcess(v.process.Pid)
    if err != nil {
        return false
    }
    // Windows'ta Signal(0) desteklenmez, process handle kontrolu gerekir
    _ = p
    return true // Basit yaklasiim: process referansi varsa calisiyordur
}

// writeConfig, acremote.ini dosyasini one-time password ile yazar.
func (v *VNCServer) writeConfig(password string) error {
    content := fmt.Sprintf(`[ultravnc]
PortNumber=5900
AllowLoopback=0
LoopbackOnly=0
passwd=%s
passwd2=%s
AuthRequired=1
ConnectPriority=0
InputsEnabled=1
DebugMode=0
DebugLevel=0
UseRegistry=0
UseDDEngine=1
UseMirrorDriver=0
FileTransferEnabled=0
FTUserImpersonation=0
RemoveWallpaper=0
RemoveAero=0
`, password, password)

    return os.WriteFile(v.configPath, []byte(content), 0600)
}

// cleanConfig, config dosyasindaki sifreleri temizler.
func (v *VNCServer) cleanConfig() {
    _ = os.Remove(v.configPath)
}

// exeDirectory, calisan exe'nin dizinini dondurur.
func exeDirectory() string {
    exe, _ := os.Executable()
    return filepath.Dir(exe)
}

// dataDirectory, ProgramData dizinini dondurur.
func dataDirectory() string {
    pd := os.Getenv("PROGRAMDATA")
    if pd == "" {
        pd = `C:\ProgramData`
    }
    return filepath.Join(pd, "AppCenter")
}
```

##### session.go - Oturum State Machine

```go
// internal/remotesupport/session.go
package remotesupport

import (
    "context"
    "fmt"
    "log"
    "sync"
    "time"

    "appcenter-agent/internal/api"
    "appcenter-agent/internal/ipc"
)

// SessionState, uzak destek oturumunun mevcut durumu.
type SessionState string

const (
    StateIdle       SessionState = "idle"
    StatePending    SessionState = "pending_approval"
    StateApproved   SessionState = "approved"
    StateConnecting SessionState = "connecting"
    StateActive     SessionState = "active"
    StateEnding     SessionState = "ending"
)

// SessionRequest, heartbeat'ten gelen uzak destek istegi.
type SessionRequest struct {
    SessionID   int    `json:"session_id"`
    AdminName   string `json:"admin_name"`
    Reason      string `json:"reason"`
    RequestedAt string `json:"requested_at"`
    TimeoutAt   string `json:"timeout_at"`
}

// SessionManager, tek seferlik bir uzak destek oturumunu yonetir.
type SessionManager struct {
    mu             sync.Mutex
    state          SessionState
    sessionID      int
    pendingRequest *SessionRequest  // Tray poll'unda dondurulur

    vnc        *VNCServer
    client     *api.Client
    logger     *log.Logger
    approvalCh chan bool           // Tray IPC'den gelen onay/red
    cancelFn   context.CancelFunc
}

// NewSessionManager olusturur.
// approvalCh buffered olmali (1) ki tray cevabi bloklamasin.


// NewSessionManager olusturur.
func NewSessionManager(client *api.Client, logger *log.Logger) *SessionManager {
    return &SessionManager{
        state:  StateIdle,
        vnc:    NewVNCServer(logger),
        client: client,
        logger: logger,
    }
}

// IsActive, bir oturumun devam edip etmedigini bildirir.
func (sm *SessionManager) IsActive() bool {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    return sm.state != StateIdle
}

// CurrentSessionID, aktif oturum ID'sini dondurur (0 = yok).
func (sm *SessionManager) CurrentSessionID() int {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    return sm.sessionID
}

// HandleRequest, heartbeat'ten gelen uzak destek istegini isler.
//
// ONAY MEKANIZMASI (Hibrit Yaklasim):
//
// Service SYSTEM hesabiyla calisir, kullanici masaustunde dogrudan UI gosteremez.
// Tray ise kullanici oturumunda calisir ama service'ten push mesaj alamaz
// (IPC modeli: tray sorar, service cevaplar).
//
// Cozum:
//   1. Service heartbeat'ten istegi alir → pending flag tutar
//   2. Tray her 3 saniyede "remote_support_status" IPC poll'u yapar
//   3. Tray pending gorurse → zengin onay dialog'u gosterir (ozel UI)
//   4. Tray CALISMIYORSA (son IPC poll > 10 sn) → Service, WTSSendMessage
//      API ile kullanici oturumunda fallback MessageBox gosterir
//   5. Onay/red sonucu → Service, server'a bildirir
//
// Neden bu yaklasim:
//   - Tray varsa: guzel, ozellestirilebilir dialog (admin adi, sebep, geri sayim)
//   - Tray yoksa: basit ama calisan Windows MessageBox (guvenlik agi)
//   - Mevcut IPC modeli degismez (tray hala soran taraf)
//   - Polling gecikmesi: 0-3 sn (kabul edilebilir)
//
func (sm *SessionManager) HandleRequest(ctx context.Context, req SessionRequest) {
    sm.mu.Lock()
    if sm.state != StateIdle {
        sm.mu.Unlock()
        sm.logger.Printf("remote support: ignoring request %d (already in state %s)", req.SessionID, sm.state)
        return
    }
    sm.state = StatePending
    sm.sessionID = req.SessionID
    sm.pendingRequest = &req
    sm.mu.Unlock()

    sm.logger.Printf("remote support: received request %d from %s", req.SessionID, req.AdminName)

    // Tray'in poll edip dialog gostermesini bekle (max 10 sn).
    // Tray 3 sn aralikla poll yapar, 10 sn icinde cevap gelmezse
    // fallback olarak WTSSendMessage kullan.
    approved := sm.waitForApproval(req, 10*time.Second)

    // Sonucu server'a bildir
    if err := sm.client.ApproveRemoteSession(req.SessionID, approved); err != nil {
        sm.logger.Printf("remote support: approve report failed: %v", err)
        sm.reset()
        return
    }

    if !approved {
        sm.logger.Printf("remote support: session %d rejected by user", req.SessionID)
        sm.reset()
        return
    }

    sm.logger.Printf("remote support: session %d approved by user", req.SessionID)
    sm.startVNC(ctx, req.SessionID)
}

// waitForApproval, onay cevabini bekler.
// Tray IPC ile cevap verirse onu kullanir.
// Tray 10 sn icinde cevap vermezse WTSSendMessage fallback'i kullanir.
func (sm *SessionManager) waitForApproval(req SessionRequest, trayTimeout time.Duration) bool {
    // approvalCh, tray'den IPC ile gelen onay cevabini tasir.
    // Tray "remote_support_status" poll'unda pending gorur,
    // dialog gosterir ve "remote_support_respond" IPC ile cevap gonderir.
    // IPC handler bu channel'a yazar.
    select {
    case approved := <-sm.approvalCh:
        sm.logger.Printf("remote support: approval from tray: %v", approved)
        return approved
    case <-time.After(trayTimeout):
        sm.logger.Printf("remote support: tray did not respond in %v, falling back to WTSSendMessage", trayTimeout)
        approved, err := ShowApprovalDialogFromService(req.AdminName, req.Reason, 120)
        if err != nil {
            sm.logger.Printf("remote support: WTSSendMessage fallback failed: %v", err)
            return false
        }
        return approved
    }
}

// startVNC, VNC server'i baslatir ve server'a hazir oldugunu bildirir.
func (sm *SessionManager) startVNC(ctx context.Context, sessionID int) {
    sm.mu.Lock()
    sm.state = StateApproved
    sm.mu.Unlock()

    // Server'dan VNC bilgilerini al (approve response'unda donmustu)
    approveResp, err := sm.client.GetRemoteSessionInfo(sessionID)
    if err != nil {
        sm.logger.Printf("remote support: get session info failed: %v", err)
        sm.cleanup(sessionID, "error")
        return
    }

    // VNC helper mevcut mu kontrol et
    if !sm.vnc.Available() {
        sm.logger.Printf("remote support: VNC helper not available")
        sm.cleanup(sessionID, "error")
        return
    }

    // VNC server'i baslat
    if err := sm.vnc.Start(approveResp.VNCPassword, approveResp.GuacdHost, approveResp.GuacdPort); err != nil {
        sm.logger.Printf("remote support: VNC start failed: %v", err)
        sm.cleanup(sessionID, "error")
        return
    }

    // Server'a hazir oldugunu bildir
    if err := sm.client.ReportRemoteReady(sessionID); err != nil {
        sm.logger.Printf("remote support: ready report failed: %v", err)
        sm.vnc.Stop()
        sm.cleanup(sessionID, "error")
        return
    }

    sm.mu.Lock()
    sm.state = StateActive
    sm.mu.Unlock()

    sm.logger.Printf("remote support: session %d active", sessionID)

    // Oturum sonlanma sinyalini bekle (context cancel ile)
    sessionCtx, cancel := context.WithCancel(ctx)
    sm.mu.Lock()
    sm.cancelFn = cancel
    sm.mu.Unlock()

    <-sessionCtx.Done()

    // Oturum sona erdi - VNC'yi kapat
    sm.vnc.Stop()
    sm.logger.Printf("remote support: session %d ended", sessionID)
    sm.reset()
}

// EndSession, aktif oturumu sonlandirir (kullanici veya server tarafindan).
func (sm *SessionManager) EndSession(endedBy string) {
    sm.mu.Lock()
    sessionID := sm.sessionID
    cancelFn := sm.cancelFn
    sm.mu.Unlock()

    if sessionID == 0 {
        return
    }

    sm.logger.Printf("remote support: ending session %d (by %s)", sessionID, endedBy)

    // VNC'yi durdur
    sm.vnc.Stop()

    // Server'a bildir
    _ = sm.client.ReportRemoteEnded(sessionID, endedBy)

    // Context'i iptal et (goroutine'leri temizle)
    if cancelFn != nil {
        cancelFn()
    }

    sm.reset()
}

// HandleEndSignal, heartbeat'ten gelen oturum sonlandirma sinyalini isler.
func (sm *SessionManager) HandleEndSignal(sessionID int) {
    sm.mu.Lock()
    if sm.sessionID != sessionID {
        sm.mu.Unlock()
        return
    }
    sm.mu.Unlock()

    sm.EndSession("admin")
}

// cleanup, hata durumunda state'i sifirlar ve server'a bildirir.
func (sm *SessionManager) cleanup(sessionID int, endedBy string) {
    _ = sm.client.ReportRemoteEnded(sessionID, endedBy)
    sm.reset()
}

// reset, state'i baslangica dondurur.
func (sm *SessionManager) reset() {
    sm.mu.Lock()
    defer sm.mu.Unlock()
    sm.state = StateIdle
    sm.sessionID = 0
    sm.cancelFn = nil
}
```

#### 8.3.2 API Client Degisiklikleri

Dosya: `internal/api/client.go` (mevcut, degisiklik)

Eklenecek metodlar:

```go
// ─── Remote Support API Metodlari ───

// ApproveRemoteSessionResponse, onay sonrasi server'dan donen bilgiler.
type ApproveRemoteSessionResponse struct {
    VNCPassword string `json:"vnc_password"`
    GuacdHost   string `json:"guacd_host"`
    GuacdPort   int    `json:"guacd_reverse_port"`
}

// ApproveRemoteSession, kullanici onay/red bildirimini gonderir.
func (c *Client) ApproveRemoteSession(sessionID int, approved bool) error {
    url := fmt.Sprintf("%s/api/v1/agent/remote-support/%d/approve", c.baseURL, sessionID)
    body := map[string]bool{"approved": approved}
    _, err := c.doJSON("POST", url, body)
    return err
}

// GetRemoteSessionInfo, onaylanan oturumun VNC bilgilerini alir.
// (approve response'unda donuyorsa ayri cagri gerekmez - approve response'u kullan)
func (c *Client) GetRemoteSessionInfo(sessionID int) (*ApproveRemoteSessionResponse, error) {
    // Bu bilgiler ApproveRemoteSession response'unda da donebilir.
    // Alternatif: approve cagrisinin response body'sini parse et.
    return nil, nil // Implement based on actual API response
}

// ReportRemoteReady, VNC baglantisinin hazir oldugunu bildirir.
func (c *Client) ReportRemoteReady(sessionID int) error {
    url := fmt.Sprintf("%s/api/v1/agent/remote-support/%d/ready", c.baseURL, sessionID)
    body := map[string]any{"vnc_ready": true, "local_vnc_port": 5900}
    _, err := c.doJSON("POST", url, body)
    return err
}

// ReportRemoteEnded, oturumun sonlandigini bildirir.
func (c *Client) ReportRemoteEnded(sessionID int, endedBy string) error {
    url := fmt.Sprintf("%s/api/v1/agent/remote-support/%d/ended", c.baseURL, sessionID)
    body := map[string]string{"ended_by": endedBy}
    _, err := c.doJSON("POST", url, body)
    return err
}
```

#### 8.3.3 IPC Degisiklikleri

Dosya: `internal/ipc/namedpipe.go` (mevcut, degisiklik)

Request struct'a `Data` alani eklenir:

```go
type Request struct {
    Action    string `json:"action"`
    AppID     int    `json:"app_id,omitempty"`
    Timestamp string `json:"timestamp,omitempty"`
    Data      any    `json:"data,omitempty"`  // YENi: remote support request vb.
}
```

#### 8.3.4 Core.go - Heartbeat Isleme

Dosya: `cmd/service/core.go` (mevcut, degisiklik)

Mevcut `runAgent()` fonksiyonuna ek:

```go
// SessionManager olustur
sessionMgr := remotesupport.NewSessionManager(client, logger)

// Heartbeat sonuclarini isle (mevcut task isleme dongusu icinde)
// PollResult struct'ina RemoteSupportRequest alani eklenmis olmali
go func() {
    for result := range pollResults {
        // ... mevcut task isleme kodu ...

        // Remote support request kontrolu
        if result.RemoteSupportRequest != nil {
            go sessionMgr.HandleRequest(ctx, *result.RemoteSupportRequest)
        }

        // Remote support end sinyali
        if result.RemoteSupportEnd != nil {
            sessionMgr.HandleEndSignal(result.RemoteSupportEnd.SessionID)
        }
    }
}()

// IPC handler'a remote support endpoint'lerini ekle
// buildIPCHandler fonksiyonuna:
//   "remote_support_status" → sessionMgr.IsActive(), CurrentSessionID()
//   "remote_support_end"    → sessionMgr.EndSession("user")
```

IPC handler genisletme:

```go
func buildIPCHandler(
    client *api.Client,
    cfg *config.Config,
    taskQueue *queue.TaskQueue,
    logger *log.Logger,
    serviceStarted time.Time,
    sessionMgr *remotesupport.SessionManager, // YENi parametre
) ipc.Handler {
    return func(req ipc.Request) ipc.Response {
        switch req.Action {
        // ... mevcut case'ler ...

        case "remote_support_status":
            return ipc.Response{
                Status: "ok",
                Data: map[string]any{
                    "active":     sessionMgr.IsActive(),
                    "session_id": sessionMgr.CurrentSessionID(),
                },
            }

        case "remote_support_end":
            sessionMgr.EndSession("user")
            return ipc.Response{Status: "ok", Message: "Session end requested"}

        default:
            return ipc.Response{Status: "error", Message: "unknown action"}
        }
    }
}
```

#### 8.3.5 Heartbeat Degisiklikleri

Dosya: `internal/heartbeat/heartbeat.go` (mevcut, degisiklik)

PollResult struct'ina ek:

```go
// PollResult, heartbeat response'undan parse edilen bilgiler.
type PollResult struct {
    Commands             []TaskCommand         // mevcut
    Config               *HeartbeatConfig      // mevcut
    RemoteSupportRequest *RemoteSupportRequest // YENi
    RemoteSupportEnd     *RemoteSupportEnd     // YENi
}

type RemoteSupportRequest struct {
    SessionID   int    `json:"session_id"`
    AdminName   string `json:"admin_name"`
    Reason      string `json:"reason"`
    RequestedAt string `json:"requested_at"`
    TimeoutAt   string `json:"timeout_at"`
}

type RemoteSupportEnd struct {
    SessionID int `json:"session_id"`
}
```

Heartbeat response parse'ina ek:

```go
// Mevcut parseHeartbeatResponse fonksiyonunda:
if rsReq, ok := respBody["remote_support_request"]; ok && rsReq != nil {
    var req RemoteSupportRequest
    if b, _ := json.Marshal(rsReq); json.Unmarshal(b, &req) == nil {
        result.RemoteSupportRequest = &req
    }
}

if rsEnd, ok := respBody["remote_support_end"]; ok && rsEnd != nil {
    var end RemoteSupportEnd
    if b, _ := json.Marshal(rsEnd); json.Unmarshal(b, &end) == nil {
        result.RemoteSupportEnd = &end
    }
}
```

#### 8.3.6 Tray Degisiklikleri

Dosya: `internal/tray/systray_windows.go` (mevcut, degisiklik)

Eklenmesi gerekenler:

1. **Onay Dialog'u:**

```go
// internal/tray/approval_windows.go (YENi dosya)
//go:build windows

package tray

import (
    "fmt"
    "syscall"
    "unsafe"
)

var (
    user32           = syscall.NewLazyDLL("user32.dll")
    procMessageBoxW  = user32.NewProc("MessageBoxW")
)

const (
    MB_YESNO        = 0x00000004
    MB_ICONQUESTION = 0x00000020
    MB_TOPMOST      = 0x00040000
    MB_SETFOREGROUND = 0x00010000
    IDYES           = 6
)

// ShowApprovalDialog, kullaniciya onay dialog'u gosterir.
// Bloklayan bir cagridir - kullanici cevap verene kadar bekler.
func ShowApprovalDialog(adminName, reason string) bool {
    title, _ := syscall.UTF16PtrFromString("AppCenter - Uzak Destek İsteği")
    message, _ := syscall.UTF16PtrFromString(fmt.Sprintf(
        "%s bilgisayarınıza uzaktan bağlanmak istiyor.\n\n"+
            "Sebep: %s\n\n"+
            "Bağlantıya izin vermek istiyor musunuz?",
        adminName, reason,
    ))

    ret, _, _ := procMessageBoxW.Call(
        0,
        uintptr(unsafe.Pointer(message)),
        uintptr(unsafe.Pointer(title)),
        uintptr(MB_YESNO|MB_ICONQUESTION|MB_TOPMOST|MB_SETFOREGROUND),
    )

    return ret == IDYES
}
```

2. **Remote Support Polling (3 sn) + Onay Dialog + Ikon Degisimi:**

Mevcut 15 sn status polling'e EK olarak, 3 sn aralikla remote support polling yapilir.
Bu polling hem onay istegini yakalar hem de aktif oturum durumunu gosterir.

```go
// systray_windows.go icinde onReady fonksiyonuna ek goroutine:

// Remote support polling (3 sn) - status polling'den bagimsiz
go func() {
    ticker := time.NewTicker(3 * time.Second)
    defer ticker.Stop()
    for range ticker.C {
        a.checkRemoteSupport()
    }
}()

func (a *App) checkRemoteSupport() {
    resp, err := a.ipc.Send(ipc.NewRequest("remote_support_status", 0))
    if err != nil || resp == nil || resp.Status != "ok" {
        return
    }

    payload, _ := json.Marshal(resp.Data)
    var rs struct {
        State          string          `json:"state"`
        SessionID      int             `json:"session_id"`
        PendingRequest *json.RawMessage `json:"pending_request"` // null veya SessionRequest
    }
    if err := json.Unmarshal(payload, &rs); err != nil {
        return
    }

    switch rs.State {
    case "pending_approval":
        // Onay dialog goster (bir kez - tekrar gostermemek icin flag tut)
        if a.remoteDialogShown {
            return
        }
        a.remoteDialogShown = true

        var req struct {
            AdminName string `json:"admin_name"`
            Reason    string `json:"reason"`
        }
        if rs.PendingRequest != nil {
            json.Unmarshal(*rs.PendingRequest, &req)
        }

        // Dialog BLOKLAYAN cagri - ayri goroutine'de calistir
        go func() {
            approved := ShowApprovalDialog(req.AdminName, req.Reason)
            // Cevabi service'e IPC ile bildir
            a.ipc.Send(ipc.Request{
                Action: "remote_support_respond",
                Data:   map[string]bool{"approved": approved},
            })
            a.remoteDialogShown = false
        }()

    case "active":
        // Ikon kirmizi, tooltip guncelle
        a.setIconState("remote_active")
        systray.SetTooltip("AppCenter Agent - Uzak destek oturumu aktif")
        mEndRemote.Enable()  // "Uzak Destegi Bitir" menusu aktif

    default: // idle
        if a.lastRemoteState == "active" {
            mEndRemote.Disable()
        }
    }

    a.lastRemoteState = rs.State
}
```

**Service tarafindaki IPC handler (core.go) guncellenmis hali:**

```go
case "remote_support_status":
    // Tray bu endpoint'i 3 sn aralikla poll eder
    sm.mu.Lock()
    data := map[string]any{
        "state":      string(sm.state),
        "session_id": sm.sessionID,
    }
    if sm.pendingRequest != nil {
        data["pending_request"] = sm.pendingRequest
    }
    sm.lastTrayPollAt = time.Now() // Tray'in hayatta oldugunu biliyoruz
    sm.mu.Unlock()
    return ipc.Response{Status: "ok", Data: data}

case "remote_support_respond":
    // Tray'den onay/red cevabi geldi
    var resp struct {
        Approved bool `json:"approved"`
    }
    if b, _ := json.Marshal(req.Data); json.Unmarshal(b, &resp) == nil {
        // approvalCh'e yaz (waitForApproval bunu bekliyor)
        select {
        case sessionMgr.approvalCh <- resp.Approved:
        default:
        }
    }
    return ipc.Response{Status: "ok"}
```
```

3. **Tray Menusune "Uzak Desteği Bitir" Secenegi:**

```go
// onReady fonksiyonunda:
mEndRemote := systray.AddMenuItem("Uzak Desteği Bitir", "Aktif uzak destek oturumunu sonlandır")
mEndRemote.Disable() // Varsayilan: devre disi

// Menu click handler'da:
case <-mEndRemote.ClickedCh:
    a.ipc.Send(ipc.NewRequest("remote_support_end", 0))
```

4. **IPC'den Gelen Onay Istegi Handler:**

Tray, IPC uzerinden `remote_support_request` mesaji aldiginda:

```go
// Service tarafinda IPC handler bu mesaji gonderecek:
// Tray tarafinda, eger service IPC yerine dogrudan tray'e mesaj gonderiyorsa
// bu handler kullanilir.
//
// Ancak mevcut mimaride IPC tek yonlu (tray → service istek yapar).
// Onerilen yaklasim: Tray periyodik olarak "remote_support_status" poll yapar
// ve pending_approval gorurse dialog gosterir.

// ALTERNATIF: Service, Named Pipe server'inda bekleyen tray client'a
// push mesaj gonderemez (pipe request-response modelinde calisir).
//
// COZUM: Heartbeat'ten gelen remote_support_request'i service alir,
// service bir "pending" flag tutar. Tray 15 saniyede bir poll edince
// bu flag'i gorur ve dialog gosterir. Ama 15 sn bekleme cok uzun.
//
// DAHA IYI COZUM: Service, remote_support_request aldiginda
// tray'e Windows mesaji gonderir (WM_USER tabanlı) veya
// ayri bir "notification" named pipe acar.
//
// EN BASIT COZUM: Service, onay dialog'unu KENDISI gosterir.
// Service SYSTEM olarak calisir ama `WTSSendMessage` API'si ile
// aktif kullanici oturumunda dialog gosterebilir.

// WTSSendMessage yaklasimi:
// internal/remotesupport/dialog_windows.go
```

**Onay Dialog Stratejisi (onerilen):**

Service SYSTEM hesabiyla calisiyor, tray kullanici oturumunda. Iki secenek:

**Secenek A (Onerilen): WTSSendMessage**
Service, `WTSSendMessage` Windows API'sini kullanarak aktif kullanici oturumunda dogrudan dialog gosterir. Bu API SYSTEM hesabindan cagrilabilir ve kullanici oturumunda gorunur.

```go
// internal/remotesupport/dialog_windows.go
//go:build windows

package remotesupport

import (
    "fmt"
    "syscall"
    "unsafe"
)

var (
    wtsapi32              = syscall.NewLazyDLL("wtsapi32.dll")
    procWTSSendMessageW   = wtsapi32.NewProc("WTSSendMessageW")
    kernel32              = syscall.NewLazyDLL("kernel32.dll")
    procWTSGetActiveConsoleSessionId = kernel32.NewProc("WTSGetActiveConsoleSessionId")
)

const (
    WTS_CURRENT_SERVER_HANDLE = 0
    MB_YESNO_WTS              = 0x00000004
    IDYES_WTS                 = 6
)

// ShowApprovalDialogFromService, SYSTEM hesabindan aktif kullaniciya
// dialog gosterir ve cevabini bekler.
func ShowApprovalDialogFromService(adminName, reason string, timeoutSec int) (bool, error) {
    sessionID, _, _ := procWTSGetActiveConsoleSessionId.Call()
    if sessionID == 0xFFFFFFFF {
        return false, fmt.Errorf("no active console session")
    }

    title, _ := syscall.UTF16FromString("AppCenter - Uzak Destek İsteği")
    message, _ := syscall.UTF16FromString(fmt.Sprintf(
        "%s bilgisayarınıza uzaktan bağlanmak istiyor.\n\n"+
            "Sebep: %s\n\n"+
            "Bağlantıya izin vermek istiyor musunuz?",
        adminName, reason,
    ))

    var response uint32
    var bWait int32 = 1 // TRUE - cevap bekle

    ret, _, err := procWTSSendMessageW.Call(
        WTS_CURRENT_SERVER_HANDLE,
        sessionID,
        uintptr(unsafe.Pointer(&title[0])),
        uintptr(len(title)*2),
        uintptr(unsafe.Pointer(&message[0])),
        uintptr(len(message)*2),
        uintptr(MB_YESNO_WTS),
        uintptr(timeoutSec),
        uintptr(unsafe.Pointer(&response)),
        uintptr(bWait),
    )

    if ret == 0 {
        return false, fmt.Errorf("WTSSendMessage failed: %v", err)
    }

    return response == IDYES_WTS, nil
}
```

**Secenek B (Yedek): Tray IPC poll**
Tray her 3 saniyede `remote_support_status` poll yapar, pending gorurse kendi dialog'unu gosterir. Daha yavas ama mimari olarak daha temiz.

**Karar:** Secenek A ile basla (WTSSendMessage). Calismazsa Secenek B'ye gec.

#### 8.3.7 Config Degisiklikleri

Dosya: `internal/config/config.go` (mevcut, degisiklik)

```go
type Config struct {
    // ... mevcut alanlar ...
    RemoteSupport RemoteSupportConfig `yaml:"remote_support"`
}

type RemoteSupportConfig struct {
    Enabled           bool   `yaml:"enabled"`
    VNCHelperPath     string `yaml:"vnc_helper_path"`      // bos ise exe dizininde arar
    GuacdReversePort  int    `yaml:"guacd_reverse_port"`   // heartbeat ile override edilebilir
    ApprovalTimeoutSec int   `yaml:"approval_timeout_sec"` // varsayilan 120
}
```

Dosya: `configs/config.yaml.template` (mevcut, degisiklik)

```yaml
# ... mevcut alanlar ...

remote_support:
  enabled: true
  vnc_helper_path: ""        # bos ise exe dizininde arar
  guacd_reverse_port: 5500   # heartbeat config ile override edilebilir
  approval_timeout_sec: 120
```

### 8.6 MSI Degisiklikleri (1 gun)

WiX dosyasina eklenmesi gerekenler:

1. `acremote-helper.exe` → `C:\Program Files\AppCenter\acremote-helper.exe`
2. `acremote.ini.template` → `C:\ProgramData\AppCenter\acremote.ini` (NeverOverwrite)

```xml
<!-- WiX Product.wxs icinde, mevcut component group'a ek -->
<Component Id="ACRemoteHelper" Guid="YENI-GUID">
    <File Id="ACRemoteHelperExe"
          Source="$(var.BuildDir)\acremote-helper.exe"
          KeyPath="yes" />
</Component>
```

MSI build pipeline'inda `acremote-helper.exe`'yi GitHub Releases'tan veya artifact'tan indirme adimi eklenir.

---

## Dosya Degisiklik Ozeti

| Dosya | Islem | Aciklama |
|-------|-------|----------|
| `internal/remotesupport/vnc.go` | YENI | VNC server yasam dongusu |
| `internal/remotesupport/session.go` | YENI | Oturum state machine |
| `internal/remotesupport/dialog_windows.go` | YENI | WTSSendMessage onay dialog |
| `internal/remotesupport/dialog_nonwindows.go` | YENI | Non-Windows stub |
| `internal/api/client.go` | DEGISIKLIK | Remote support API metodlari |
| `internal/ipc/namedpipe.go` | DEGISIKLIK | Request.Data alani |
| `internal/heartbeat/heartbeat.go` | DEGISIKLIK | RemoteSupportRequest/End parse |
| `internal/tray/systray_windows.go` | DEGISIKLIK | Ikon, menu, durum gosterimi |
| `internal/config/config.go` | DEGISIKLIK | RemoteSupportConfig |
| `cmd/service/core.go` | DEGISIKLIK | SessionManager entegrasyonu |
| `configs/config.yaml.template` | DEGISIKLIK | remote_support bolumu |
| `.github/workflows/build.yml` | DEGISIKLIK | acremote-helper artifact indirme |

---

## Test Plani

### Birim Testler

```go
// internal/remotesupport/vnc_test.go
func TestVNCServerAvailable(t *testing.T)     // helper exe yoksa false
func TestWriteConfig(t *testing.T)            // ini dosyasi dogru icerigi yaziyor mu
func TestCleanConfig(t *testing.T)            // ini dosyasi siliniyor mu

// internal/remotesupport/session_test.go
func TestSessionStateTransitions(t *testing.T) // idle → pending → approved → active → idle
func TestDuplicateRequestIgnored(t *testing.T) // aktif oturum varken yeni istek reddedilir
func TestEndSession(t *testing.T)              // oturum sonlandirma

// internal/heartbeat/heartbeat_test.go (mevcut teste ek)
func TestParseRemoteSupportRequest(t *testing.T) // heartbeat response parse
```

### Manuel Test Adimlari

1. [ ] `acremote-helper.exe` agent dizininde mevcut
2. [ ] Heartbeat'te `remote_support_request` aliniyor
3. [ ] WTSSendMessage dialog gorunuyor (konsol oturumunda)
4. [ ] Onayla → VNC basliyor → reverse connection kuruluyor
5. [ ] Reddet → VNC baslamiyor → server'a rejected bildirimi
6. [ ] Oturum bitir (admin) → VNC kapaniyor → config temizleniyor
7. [ ] Oturum bitir (kullanici, tray'den) → VNC kapaniyor
8. [ ] Timeout (2dk) → VNC baslamiyor
9. [ ] Agent offline olursa → VNC kapaniyor
10. [ ] Tray ikonu: normal (yesil), remote aktif (kirmizi), offline (gri)

---

## Bagimlilk Notu

Agent tarafindaki isler server API'lerine bagimlidir. Gelistirme sirasi:

1. Server DB + API (Faz 8.2) tamamlanmali
2. Agent kodlari yazilir (Faz 8.3)
3. Calisan helper binary hazir olmali (fork/rebrand zorunlu degil)
4. Guacamole kurulumu (Faz 8.4) uctan uca test icin gerekli
5. MSI entegrasyonu (Faz 8.6) en son

---

## Modul Baslangicina Donus (Rollback) - Agent

Olası sorunda Faz 8 degisikliklerini hizla etkisizlestirmek icin:

1. Feature flag kapat:
   - `config.yaml` -> `remote_support.enabled: false`
2. Runtime temizligi:
   - `acremote-helper.exe -kill`
   - `C:\ProgramData\AppCenter\acremote.ini` sil
3. IPC fallback:
   - `remote_support_status/respond/end` actionlari "disabled" cevabi dondurur
4. Session state sifirlama:
   - `SessionManager.reset()` tetiklenir, aktif oturum no-op kapanir
5. Kod geri donus:
   - Remote support commit serisi geri alininca agent Faz 7 davranisina geri doner
