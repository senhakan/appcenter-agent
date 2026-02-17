//go:build windows

package system

import (
	"context"
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

func GetLoggedInSessions() []LoggedInSession {
	// query user (quser) reflects interactive terminal sessions and includes state
	// (Active/Disc). This avoids "ghost" logon sessions that Win32_LogonSession may
	// still report even when there is no current interactive session.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "cmd.exe", "/c", "query user").Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	lines := strings.Split(string(out), "\n")
	items := make([]LoggedInSession, 0, 4)
	seen := make(map[string]struct{}, 4)

	isActive := func(state string) bool {
		switch strings.ToLower(strings.TrimSpace(state)) {
		case "active", "aktif", "etkin":
			return true
		default:
			return false
		}
	}

	for i, line := range lines {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		// Skip header line.
		if i == 0 && strings.Contains(strings.ToLower(line), "username") {
			continue
		}
		line = strings.TrimLeft(line, ">")
		fields := strings.Fields(line)
		// Expected (at least): USERNAME SESSIONNAME ID STATE ...
		if len(fields) < 4 {
			continue
		}

		username := fields[0]
		sessionName := fields[1]
		state := fields[3]
		if username == "" || !isActive(state) {
			continue
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}

		sType := "local"
		if strings.HasPrefix(strings.ToLower(sessionName), "rdp-tcp") {
			sType = "rdp"
		}

		items = append(items, LoggedInSession{
			Username:    username,
			SessionType: sType,
		})
	}
	return items
}
