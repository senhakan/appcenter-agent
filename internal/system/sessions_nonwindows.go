//go:build !windows

package system

// LoggedInSession represents a currently logged-in user session on the machine.
// SessionType is expected to be "local" or "rdp".
type LoggedInSession struct {
	Username    string `json:"username"`
	SessionType string `json:"session_type"`
	LogonID     string `json:"logon_id,omitempty"`
}

// GetLoggedInSessions returns active logged-in sessions. Non-Windows platforms
// return an empty list.
func GetLoggedInSessions() []LoggedInSession {
	return nil
}

