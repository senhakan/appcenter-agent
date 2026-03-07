//go:build windows

package system

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
)

type psServiceRow struct {
	Name        string `json:"Name"`
	DisplayName string `json:"DisplayName"`
	State       string `json:"State"`
	StartMode   string `json:"StartMode"`
	ProcessID   int    `json:"ProcessId"`
	StartName   string `json:"StartName"`
	Description string `json:"Description"`
}

func normalizeServiceStatus(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "running":
		return "running"
	case "stopped":
		return "stopped"
	case "paused":
		return "paused"
	default:
		return "unknown"
	}
}

func normalizeStartupType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "auto":
		return "auto"
	case "manual":
		return "manual"
	case "disabled":
		return "disabled"
	default:
		return "unknown"
	}
}

func CollectServices() ([]ServiceInfo, error) {
	cmd := exec.Command(
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command",
		"Get-CimInstance Win32_Service | Select-Object Name,DisplayName,State,StartMode,ProcessId,StartName,Description | ConvertTo-Json -Compress",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	txt := strings.TrimSpace(string(out))
	if txt == "" {
		return []ServiceInfo{}, nil
	}
	var rows []psServiceRow
	if strings.HasPrefix(txt, "[") {
		if err := json.Unmarshal(out, &rows); err != nil {
			return nil, err
		}
	} else {
		var one psServiceRow
		if err := json.Unmarshal(out, &one); err != nil {
			return nil, err
		}
		rows = []psServiceRow{one}
	}
	items := make([]ServiceInfo, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		items = append(items, ServiceInfo{
			Name:        name,
			DisplayName: strings.TrimSpace(r.DisplayName),
			Status:      normalizeServiceStatus(r.State),
			StartupType: normalizeStartupType(r.StartMode),
			PID:         r.ProcessID,
			RunAs:       strings.TrimSpace(r.StartName),
			Description: strings.TrimSpace(r.Description),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}
