package utils

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoggerRotates(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "agent.log")

	logger, closer, err := NewLogger(logPath, 1, 2) // 1MB
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer closer.Close()

	// Write > 1MB to force rotation.
	chunk := bytes.Repeat([]byte("a"), 64*1024)
	for i := 0; i < 20; i++ {
		logger.Println(string(chunk))
	}
	_ = closer.Close()

	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("log file missing: %v", err)
	}
	// At least one rotated file should exist.
	if _, err := os.Stat(logPath + ".1"); err != nil {
		t.Fatalf("expected rotated file .1 to exist: %v", err)
	}
}
