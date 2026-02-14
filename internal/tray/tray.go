package tray

import (
	"fmt"
	"strings"
	"time"

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
	ID          int    `json:"id"`
	DisplayName string `json:"display_name"`
	Version     string `json:"version"`
	Installed   bool   `json:"installed"`
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
