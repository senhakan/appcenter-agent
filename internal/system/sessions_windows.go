//go:build windows

package system

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// LoggedInSession represents a currently logged-in user session on the machine.
// SessionType is expected to be "local" or "rdp".
type LoggedInSession struct {
	Username    string `json:"username"`
	SessionType string `json:"session_type"`
	LogonID     string `json:"logon_id,omitempty"`
}

type psSession struct {
	Username  string `json:"Username"`
	LogonID   string `json:"LogonId"`
	LogonType int    `json:"LogonType"`
}

func GetLoggedInSessions() []LoggedInSession {
	// Prefer WMI/CIM over parsing localized CLI output.
	// LogonType mapping (Windows):
	// - 2  = Interactive (console/local)
	// - 10 = RemoteInteractive (RDP)
	ps := strings.Join([]string{
		"$ErrorActionPreference='SilentlyContinue';",
		"$sessions = Get-CimInstance Win32_LogonSession | Where-Object { $_.LogonType -in 2,10 };",
		"$out = @();",
		"foreach($s in $sessions){",
		"  $accounts = Get-CimAssociatedInstance -InputObject $s -Association Win32_LoggedOnUser;",
		"  foreach($u in $accounts){",
		"    if(-not $u){ continue }",
		"    $name = $u.Name;",
		"    $domain = $u.Domain;",
		"    if(-not $name){ continue }",
		"    if($name -match '^(DWM-|UMFD-|SYSTEM|LOCAL SERVICE|NETWORK SERVICE)$'){ continue }",
		"    $full = $name; if($domain){ $full = \"$domain\\$name\" }",
		"    $out += [pscustomobject]@{ Username=$full; LogonId=[string]$s.LogonId; LogonType=[int]$s.LogonType };",
		"  }",
		"}",
		"if($out.Count -eq 0){ '[]' } else { $out | Sort-Object Username -Unique | ConvertTo-Json -Compress }",
	}, " ")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps).Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	// ConvertTo-Json returns either an object or an array depending on count.
	var many []psSession
	if err := json.Unmarshal(out, &many); err != nil {
		var one psSession
		if err2 := json.Unmarshal(out, &one); err2 != nil {
			return nil
		}
		many = []psSession{one}
	}

	seen := make(map[string]struct{}, len(many))
	items := make([]LoggedInSession, 0, len(many))
	for _, s := range many {
		u := strings.TrimSpace(s.Username)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}

		st := ""
		switch s.LogonType {
		case 2:
			st = "local"
		case 10:
			st = "rdp"
		default:
			continue
		}
		items = append(items, LoggedInSession{
			Username:    u,
			SessionType: st,
			LogonID:     strings.TrimSpace(s.LogonID),
		})
	}
	return items
}
