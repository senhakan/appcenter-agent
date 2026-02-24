//go:build windows

package tray

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"appcenter-agent/internal/ipc"
	"github.com/getlantern/systray"
)

type App struct {
	ipc IPCClient

	mu            sync.Mutex
	lastIconState string
}

func NewApp() *App {
	return &App{
		ipc: DefaultIPCClient{},
	}
}

func Run() error {
	release, err := ensureSingleInstance()
	if err != nil {
		return err
	}
	defer release()

	app := NewApp()
	systray.Run(app.onReady, app.onExit)
	return nil
}

func (a *App) onReady() {
	// Default to "service down" until IPC confirms service is reachable.
	a.setIconState("service_down")
	systray.SetTitle("AppCenter")
	systray.SetTooltip("AppCenter Agent")

	mRefreshStatus := systray.AddMenuItem("Durumu Yenile", "Servis durumunu güncelle")
	mStore := systray.AddMenuItem("Mağaza", "Uygulama mağazasını aç")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Çıkış", "AppCenter Tray'den çık")

	// Periodic status refresh
	go func() {
		a.refreshStatus()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			a.refreshStatus()
		}
	}()

	// Menu click handler
	go func() {
		for {
			select {
			case <-mRefreshStatus.ClickedCh:
				a.refreshStatus()
			case <-mStore.ClickedCh:
				if err := launchStoreWindowProcess(); err != nil {
					trayDiagf("launch store process failed: %v", err)
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (a *App) onExit() {}

func (a *App) refreshStatus() {
	resp, err := a.ipc.Send(ipc.NewRequest("get_status", 0))
	if err != nil || resp == nil || resp.Status != "ok" {
		a.setIconState("service_down")
		systray.SetTitle("AppCenter (çevrimdışı)")
		systray.SetTooltip("AppCenter Agent - servise ulaşılamıyor")
		return
	}

	payload, _ := json.Marshal(resp.Data)
	var s StatusSnapshot
	if err := json.Unmarshal(payload, &s); err != nil {
		a.setIconState("service_down")
		systray.SetTooltip("AppCenter Agent - durum okunamadı")
		return
	}

	serverOK, _ := CheckServerHealth()
	if serverOK {
		a.setIconState("ok")
	} else {
		a.setIconState("server_down")
	}

	systray.SetTitle(statusTitle(s))
	tt := statusTooltip(s)
	if !serverOK {
		tt += " (sunucuya ulaşılamıyor)"
	}
	systray.SetTooltip(tt)
}

func (a *App) setIconState(state string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Avoid redundant SetIcon calls (systray writes icon bytes to temp files).
	if a.lastIconState == state {
		return
	}
	a.lastIconState = state

	switch state {
	case "ok":
		systray.SetIcon(connectedIconBytes())
	case "server_down":
		systray.SetIcon(disconnectedIconBytes())
	default:
		systray.SetIcon(serviceDownIconBytes())
	}
}

func launchStoreWindowProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exeDir := filepath.Dir(exe)
	storeUI := filepath.Join(exeDir, "appcenter-store-ui.exe")
	if _, err := os.Stat(storeUI); err == nil {
		return exec.Command(storeUI).Start()
	}

	// Legacy fallback if store-ui binary is missing.
	return exec.Command(exe, "open_store_legacy").Start()
}

func OpenStoreUI() error { return launchStoreWindowProcess() }
