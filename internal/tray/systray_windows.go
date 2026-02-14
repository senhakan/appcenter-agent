//go:build windows

package tray

import (
	"encoding/json"
	"fmt"
	"time"

	"appcenter-agent/internal/ipc"
	"github.com/getlantern/systray"
)

type App struct {
	ipc IPCClient
}

func NewApp() *App {
	return &App{ipc: DefaultIPCClient{}}
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
	if b := appIconBytes(); len(b) > 0 {
		systray.SetIcon(b)
	}
	systray.SetTitle("AppCenter")
	systray.SetTooltip("AppCenter Agent")

	mRefreshStatus := systray.AddMenuItem("Refresh Status", "Get service status")
	mRefreshStore := systray.AddMenuItem("Refresh Store", "Load store applications")
	mStore := systray.AddMenuItem("Store", "Store applications")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Exit", "Exit AppCenter Tray")

	storeItems := make(map[int]*systray.MenuItem)

	go func() {
		// Initial status pull
		a.refreshStatus()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			a.refreshStatus()
		}
	}()

	go func() {
		for {
			select {
			case <-mRefreshStatus.ClickedCh:
				a.refreshStatus()
			case <-mRefreshStore.ClickedCh:
				apps, err := a.getStoreApps()
				if err != nil {
					systray.SetTooltip("Store refresh failed")
					continue
				}
				for _, app := range apps {
					if _, exists := storeItems[app.ID]; exists {
						continue
					}
					item := mStore.AddSubMenuItem(appLabel(app), fmt.Sprintf("Install %s", app.DisplayName))
					storeItems[app.ID] = item
					go func(appID int, mi *systray.MenuItem) {
						for range mi.ClickedCh {
							_, _ = a.ipc.Send(ipc.NewRequest("install_from_store", appID))
							systray.SetTooltip(fmt.Sprintf("Install request queued for app_id=%d", appID))
						}
					}(app.ID, item)
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
		systray.SetTitle("AppCenter (offline)")
		systray.SetTooltip("AppCenter Agent - service unreachable")
		return
	}

	payload, _ := json.Marshal(resp.Data)
	var s StatusSnapshot
	if err := json.Unmarshal(payload, &s); err != nil {
		systray.SetTooltip("AppCenter Agent - status parse failed")
		return
	}

	systray.SetTitle(statusTitle(s))
	systray.SetTooltip(statusTooltip(s))
}

func (a *App) getStoreApps() ([]StoreApp, error) {
	resp, err := a.ipc.Send(ipc.NewRequest("get_store", 0))
	if err != nil {
		return nil, err
	}
	if resp.Status != "ok" {
		return nil, fmt.Errorf(resp.Message)
	}

	payload, _ := json.Marshal(resp.Data)
	var store StorePayload
	if err := json.Unmarshal(payload, &store); err != nil {
		return nil, err
	}
	return store.Apps, nil
}
