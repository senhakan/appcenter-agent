package system

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type SystemProfileState struct {
	LastSentAtUTC string `json:"last_sent_at_utc"`
	LastHash      string `json:"last_hash"`
}

func DefaultSystemProfileStatePath() string {
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\AppCenter\system_profile_state.json`
	}
	return "system_profile_state.json"
}

func LoadSystemProfileState(path string) (SystemProfileState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SystemProfileState{}, err
	}
	var st SystemProfileState
	if err := json.Unmarshal(b, &st); err != nil {
		return SystemProfileState{}, err
	}
	return st, nil
}

func SaveSystemProfileState(path string, st SystemProfileState) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func ParseUTC(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

