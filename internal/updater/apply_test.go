package updater

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"appcenter-agent/internal/config"
)

type stubRunner struct {
	cmd *exec.Cmd
	err error
}

func (s *stubRunner) Start(cmd *exec.Cmd) error {
	s.cmd = cmd
	return s.err
}

func TestApplyIfPendingSkipsWhenDisabled(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.Config{
		Agent:    config.AgentConfig{Version: "1.0.0"},
		Download: config.DownloadConfig{TempDir: tmp, BandwidthLimitKBs: 1024},
		Update:   config.UpdateConfig{AutoApply: false},
	}
	logger := log.New(os.Stdout, "", 0)
	r := &stubRunner{}
	if err := applyIfPending(context.Background(), cfg, "cfg.yaml", "svc.exe", logger, r); err != nil {
		t.Fatalf("err=%v", err)
	}
	if r.cmd != nil {
		t.Fatalf("helper should not start")
	}
}

func TestApplyIfPendingStartsHelperAndReturnsRestart(t *testing.T) {
	tmp := t.TempDir()
	content := []byte("new-binary")
	sum := sha256.Sum256(content)
	hash := fmt.Sprintf("sha256:%x", sum[:])
	newExe := filepath.Join(tmp, "agent-update-2.0.0.exe")
	if err := os.WriteFile(newExe, content, 0o600); err != nil {
		t.Fatalf("write new exe: %v", err)
	}

	meta := StagedUpdate{Version: "2.0.0", FilePath: newExe, Hash: hash}
	metaPath := filepath.Join(tmp, "pending_update.json")
	b, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath, b, 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	cfg := config.Config{
		Agent:    config.AgentConfig{Version: "1.0.0"},
		Download: config.DownloadConfig{TempDir: tmp, BandwidthLimitKBs: 1024},
		Update: config.UpdateConfig{
			AutoApply:   true,
			ServiceName: "AppCenterAgent",
			HelperPath:  "C:\\helper.exe",
		},
	}

	logger := log.New(os.Stdout, "", 0)
	r := &stubRunner{}
	err := applyIfPending(context.Background(), cfg, "C:\\cfg.yaml", "C:\\svc.exe", logger, r)
	if err != ErrUpdateRestart {
		t.Fatalf("err=%v, want ErrUpdateRestart", err)
	}
	if r.cmd == nil || r.cmd.Path != cfg.Update.HelperPath {
		t.Fatalf("helper cmd not started: %#v", r.cmd)
	}
}

