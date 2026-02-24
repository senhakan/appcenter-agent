package tray

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"appcenter-agent/internal/config"
	"appcenter-agent/internal/ipc"
)

type IPCClient interface {
	Send(ipc.Request) (*ipc.Response, error)
}

type DefaultIPCClient struct{}

func (DefaultIPCClient) Send(req ipc.Request) (*ipc.Response, error) {
	return ipc.SendRequest(req)
}

type StatusSnapshot struct {
	Service      string `json:"service"`
	StartedAt    string `json:"started_at"`
	PendingTasks int    `json:"pending_tasks"`
	AgentVersion string `json:"agent_version"`
	AgentUUID    string `json:"agent_uuid"`
}

type StoreApp struct {
	ID               int    `json:"id"`
	DisplayName      string `json:"display_name"`
	Version          string `json:"version"`
	Description      string `json:"description"`
	IconURL          string `json:"icon_url"`
	FileSizeMB       int    `json:"file_size_mb"`
	Category         string `json:"category"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	CanUninstall     bool   `json:"can_uninstall"`
}

type StorePayload struct {
	Apps []StoreApp `json:"apps"`
}

func statusTooltip(s StatusSnapshot) string {
	status := strings.TrimSpace(s.Service)
	if status == "" {
		status = "unknown"
	}
	return fmt.Sprintf("AppCenter Agent - %s (pending: %d)", status, s.PendingTasks)
}

func statusTitle(s StatusSnapshot) string {
	if s.Service == "running" {
		return "AppCenter"
	}
	return "AppCenter (offline)"
}

func appLabel(app StoreApp) string {
	installedMark := ""
	if app.Installed {
		installedMark = " [installed]"
	}
	if app.Version == "" {
		return fmt.Sprintf("%d - %s%s", app.ID, app.DisplayName, installedMark)
	}
	return fmt.Sprintf("%d - %s %s%s", app.ID, app.DisplayName, app.Version, installedMark)
}

func nowStamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func CheckServerHealth() (bool, string) {
	cfgPath := resolveConfigPathForTray()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return false, fmt.Sprintf("config load failed: %v", err)
	}

	base := strings.TrimRight(cfg.Server.URL, "/")
	if base == "" {
		return false, "server.url is empty"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return false, fmt.Sprintf("request build failed: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Sprintf("request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("health status=%s", resp.Status)
	}
	return true, "ok"
}

func resolveConfigPathForTray() string {
	if p := os.Getenv("APPCENTER_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\AppCenter\config.yaml`
	}
	return "configs/config.yaml.template"
}
