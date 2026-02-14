package updater

import (
	"context"
	"log"
	"os"
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
