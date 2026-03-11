package system

import "strings"

const (
	sessionStateActive       = "active"
	sessionStateDisconnected = "disconnected"
)

func normalizeSessionType(protocol uint16) string {
	if protocol == 2 {
		return "rdp"
	}
	return "local"
}

func normalizeSessionState(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case sessionStateDisconnected, "disc":
		return sessionStateDisconnected
	default:
		return sessionStateActive
	}
}
