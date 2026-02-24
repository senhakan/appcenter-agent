//go:build windows

package tray

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"appcenter-agent/internal/ipc"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

const (
	storeWinW = 460
	storeWinH = 600
)

var (
	storeMu     sync.Mutex
	storeIsOpen bool
	storeWinRef *walk.MainWindow

	procSPIW = windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")
)

type winRect struct{ Left, Top, Right, Bottom int32 }

func systemWorkArea() (winRect, bool) {
	var r winRect
	// SPI_GETWORKAREA = 0x0030
	ret, _, _ := procSPIW.Call(0x0030, 0, uintptr(unsafe.Pointer(&r)), 0) //nolint:errcheck
	return r, ret != 0
}

// openStoreWindow opens the store window. Only one instance runs at a time.
func (a *App) openStoreWindow() {
	storeMu.Lock()
	if storeIsOpen {
		ref := storeWinRef
		storeMu.Unlock()
		if ref != nil {
			ref.Synchronize(func() {
				ref.Show()
				_ = ref.BringToTop()
				_ = ref.Activate()
				_ = ref.SetFocus()
			})
		}
		return
	}
	storeIsOpen = true
	storeMu.Unlock()

	go func() {
		runtime.LockOSThread()
		defer func() {
			runtime.UnlockOSThread()
			storeMu.Lock()
			storeIsOpen = false
			storeWinRef = nil
			storeMu.Unlock()
		}()
		runStoreWindow(a)
	}()
}

func runStoreWindow(a *App) {
	apps, err := loadStoreApps(a.ipc)
	if err != nil {
		walk.MsgBox(nil, "AppCenter Store",
			"Mağaza yüklenemedi:\n\n"+err.Error(),
			walk.MsgBoxIconError|walk.MsgBoxOK)
		return
	}

	var (
		mw         *walk.MainWindow
		searchEdit *walk.LineEdit
		appsBox    *walk.Composite
	)

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    "AppCenter Store",
		MinSize:  Size{Width: storeWinW, Height: storeWinH},
		MaxSize:  Size{Width: storeWinW, Height: storeWinH},
		Layout: VBox{
			Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12},
			Spacing: 8,
		},
		Children: []Widget{
			// ── Header ──────────────────────────────────────────────────
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					Label{
						Text: fmt.Sprintf("AppCenter Store  ·  %d uygulama", len(apps)),
						Font: Font{Family: "Segoe UI", PointSize: 12, Bold: true},
					},
					HSpacer{},
				},
			},
			// ── Search bar ──────────────────────────────────────────────
			LineEdit{
				AssignTo:  &searchEdit,
				CueBanner: "Uygulama ara...",
				Font:      Font{Family: "Segoe UI", PointSize: 10},
			},
			HSeparator{},
			// ── Scrollable app list ─────────────────────────────────────
			ScrollView{
				Layout: VBox{MarginsZero: true, Spacing: 0},
				Children: []Widget{
					Composite{
						AssignTo: &appsBox,
						Layout:   VBox{MarginsZero: true, Spacing: 0},
					},
				},
			},
		},
	}.Create()); err != nil {
		trayDiagf("store window create failed: %v", err)
		walk.MsgBox(nil, "AppCenter Store",
			"Pencere acilamadi:\n\n"+err.Error(),
			walk.MsgBoxIconError|walk.MsgBoxOK)
		return
	}
	storeMu.Lock()
	storeWinRef = mw
	storeMu.Unlock()

	// Build one card per app
	type cardEntry struct {
		card *walk.Composite
		app  StoreApp
	}
	entries := make([]cardEntry, 0, len(apps))
	for i := range apps {
		card, err := buildAppCard(appsBox, apps[i], a.ipc, mw)
		if err == nil {
			entries = append(entries, cardEntry{card: card, app: apps[i]})
		}
	}

	// Live search filter
	searchEdit.TextChanged().Attach(func() {
		q := strings.ToLower(searchEdit.Text())
		for _, e := range entries {
			show := q == "" ||
				strings.Contains(strings.ToLower(e.app.DisplayName), q) ||
				strings.Contains(strings.ToLower(e.app.Description), q) ||
				strings.Contains(strings.ToLower(e.app.Category), q)
			e.card.SetVisible(show)
		}
	})

	// Position at bottom-right corner, just above the taskbar.
	if wa, ok := systemWorkArea(); ok && wa.Right > wa.Left && wa.Bottom > wa.Top {
		x := int(wa.Right) - storeWinW - 12
		y := int(wa.Bottom) - storeWinH - 12
		if x < int(wa.Left)+4 {
			x = int(wa.Left) + 4
		}
		if y < int(wa.Top)+4 {
			y = int(wa.Top) + 4
		}
		if err := mw.SetBounds(walk.Rectangle{
			X:      x,
			Y:      y,
			Width:  storeWinW,
			Height: storeWinH,
		}); err != nil {
			trayDiagf("store window set bounds failed: %v", err)
		}
	} else {
		trayDiagf("store window work area unavailable; using default position")
	}
	mw.Show()
	_ = mw.BringToTop()
	_ = mw.Activate()
	_ = mw.SetFocus()

	mw.Run()
}

func OpenStoreWindowStandalone() error {
	app := NewApp()
	runStoreWindow(app)
	return nil
}

// buildAppCard creates one app row inside the parent composite.
func buildAppCard(parent *walk.Composite, app StoreApp, ipcClient IPCClient, form walk.Form) (*walk.Composite, error) {
	card, err := walk.NewComposite(parent)
	if err != nil {
		return nil, err
	}
	vl := walk.NewVBoxLayout()
	// walk.Margins: HNear=left, VNear=top, HFar=right, VFar=bottom
	vl.SetMargins(walk.Margins{HNear: 8, VNear: 10, HFar: 8, VFar: 10})
	vl.SetSpacing(4)
	card.SetLayout(vl)

	// ── Row 1: Name · Version | [button] ─────────────────────────
	row, _ := walk.NewComposite(card)
	hl := walk.NewHBoxLayout()
	hl.SetMargins(walk.Margins{})
	hl.SetSpacing(6)
	row.SetLayout(hl)

	nameLabel, _ := walk.NewLabel(row)
	nameLabel.SetText(app.DisplayName)

	verLabel, _ := walk.NewLabel(row)
	verText := "v" + app.Version
	if app.Installed && app.InstalledVersion != "" && app.InstalledVersion != app.Version {
		verText = "v" + app.InstalledVersion + " → " + app.Version
	}
	verLabel.SetText(verText)

	// Spacer: pushes button to the right
	hs, _ := walk.NewHSpacer(row)
	_ = hs

	// File size badge
	if app.FileSizeMB > 0 {
		sizeLabel, _ := walk.NewLabel(row)
		sizeLabel.SetText(fmt.Sprintf("%d MB", app.FileSizeMB))
	}

	// Install / Installed button
	btn, _ := walk.NewPushButton(row)
	_ = btn.SetMinMaxSize(
		walk.Size{Width: 90, Height: 26},
		walk.Size{Width: 90, Height: 26},
	)

	if app.Installed {
		btn.SetText("✓ Yüklü")
		btn.SetEnabled(false)
	} else {
		btn.SetText("Kur")
		appID := app.ID
		appName := app.DisplayName

		btn.Clicked().Attach(func() {
			btn.SetEnabled(false)
			btn.SetText("Kuruluyor…")

			go func() {
				resp, err := ipcClient.Send(ipc.NewRequest("install_from_store", appID))
				btn.Synchronize(func() {
					switch {
					case err != nil:
						walk.MsgBox(form, "Kurulum Hatası",
							fmt.Sprintf("%s kurulamadı:\n%v", appName, err),
							walk.MsgBoxIconError|walk.MsgBoxOK)
						btn.SetEnabled(true)
						btn.SetText("Kur")
					case resp == nil || resp.Status != "ok":
						msg := "Bilinmeyen hata"
						if resp != nil && resp.Message != "" {
							msg = resp.Message
						}
						walk.MsgBox(form, "Kurulum Hatası",
							fmt.Sprintf("%s kurulamadı:\n%s", appName, msg),
							walk.MsgBoxIconError|walk.MsgBoxOK)
						btn.SetEnabled(true)
						btn.SetText("Kur")
					default:
						queueStatus := readQueueStatus(resp.Data)
						switch queueStatus {
						case "already_installed":
							btn.SetText("✓ Yüklü")
							btn.SetEnabled(false)
						case "already_queued":
							btn.SetText("Kuyrukta")
							btn.SetEnabled(false)
						default:
							btn.SetText("Kuyruğa alındı ✓")
							btn.SetEnabled(false)
						}
					}
				})
			}()
		})
	}

	// ── Row 2: Description ────────────────────────────────────────
	if app.Description != "" {
		descLabel, _ := walk.NewLabel(card)
		desc := app.Description
		if len([]rune(desc)) > 78 {
			desc = string([]rune(desc)[:78]) + "…"
		}
		descLabel.SetText(desc)
	}

	// ── Separator between cards ───────────────────────────────────
	walk.NewHSeparator(parent) //nolint:errcheck

	return card, nil
}

func readQueueStatus(data any) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	v, ok := m["queue_status"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func trayDiagf(format string, args ...any) {
	logDir := `C:\ProgramData\AppCenter\logs`
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return
	}
	logPath := filepath.Join(logDir, "tray-ui.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	_, _ = f.WriteString(fmt.Sprintf("%s "+format+"\n", append([]any{ts}, args...)...))
}

// loadStoreApps fetches the app list from the service via IPC.
func loadStoreApps(ipcClient IPCClient) ([]StoreApp, error) {
	resp, err := ipcClient.Send(ipc.NewRequest("get_store", 0))
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Status != "ok" {
		msg := "bilinmeyen hata"
		if resp != nil && resp.Message != "" {
			msg = resp.Message
		}
		return nil, fmt.Errorf("%s", msg)
	}

	raw, err := json.Marshal(resp.Data)
	if err != nil {
		return nil, err
	}
	var payload StorePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload.Apps, nil
}
