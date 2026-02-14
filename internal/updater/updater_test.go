package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"appcenter-agent/internal/config"
)

func TestSanitizeVersion(t *testing.T) {
	got := sanitizeVersion("1.0.0 / build:1")
	if got != "1.0.0___build_1" {
		t.Fatalf("sanitize=%q", got)
	}
}

func TestStageIfNeededSkipsWithoutConfig(t *testing.T) {
	cfg := config.Config{Agent: config.AgentConfig{Version: "1.0.0"}}
	logger := log.New(os.Stdout, "", 0)
	if err := StageIfNeeded(context.Background(), cfg, map[string]any{}, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStageIfNeededDownloadsAndWritesMetadata(t *testing.T) {
	content := []byte("dummy-agent-binary")
	sum := sha256.Sum256(content)
	expectedHash := fmt.Sprintf("sha256:%x", sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// downloader sets these headers; we don't validate values here.
		w.Header().Set("Content-Disposition", "attachment; filename=\"agent.exe\"")
		_, _ = w.Write(content)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	cfg := config.Config{
		Server: config.ServerConfig{URL: srv.URL},
		Agent: config.AgentConfig{
			Version:   "1.0.0",
			UUID:      "u1",
			SecretKey: "s1",
		},
		Download: config.DownloadConfig{TempDir: tmp, BandwidthLimitKBs: 1024},
	}

	hb := map[string]any{
		"latest_agent_version": "2.0.0",
		"agent_download_url":   srv.URL,
		"agent_hash":           expectedHash,
	}

	logger := log.New(os.Stdout, "", 0)
	if err := StageIfNeeded(context.Background(), cfg, hb, logger); err != nil {
		t.Fatalf("StageIfNeeded: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "pending_update.json")); err != nil {
		t.Fatalf("pending_update.json missing: %v", err)
	}
}
