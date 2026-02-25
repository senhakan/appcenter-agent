package runtimeupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func sha(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func TestSyncOnceUpdatesMissingFiles(t *testing.T) {
	trayBody := "tray-v2"
	helperBody := "helper-v2"

	mf := Manifest{
		Version: "1",
		Files: map[string]ManifestFile{
			"appcenter-tray.exe": {SHA256: "sha256:" + sha(trayBody)},
			"rshelper.exe":       {SHA256: "sha256:" + sha(helperBody)},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			_ = json.NewEncoder(w).Encode(mf)
		case "/appcenter-tray.exe":
			_, _ = w.Write([]byte(trayBody))
		case "/rshelper.exe":
			_, _ = w.Write([]byte(helperBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	m := NewManager(dir, log.New(ioDiscard{}, "", 0), nil)
	cfg := Config{BaseURL: srv.URL, IntervalMin: 60, JitterSec: 0}
	if err := m.syncOnce(context.Background(), cfg); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}

	for _, name := range []string{"appcenter-tray.exe", "rshelper.exe"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s missing after sync: %v", name, err)
		}
	}
}

func TestSyncOnceSkipsWhenHashMatches(t *testing.T) {
	content := "tray-v1"
	dir := t.TempDir()
	p := filepath.Join(dir, "appcenter-tray.exe")
	if err := os.WriteFile(p, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	mf := Manifest{
		Version: "1",
		Files: map[string]ManifestFile{
			"appcenter-tray.exe": {SHA256: sha(content)},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			_ = json.NewEncoder(w).Encode(mf)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	m := NewManager(dir, log.New(ioDiscard{}, "", 0), nil)
	cfg := Config{BaseURL: srv.URL, IntervalMin: 60, JitterSec: 0}
	if err := m.syncOnce(context.Background(), cfg); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
