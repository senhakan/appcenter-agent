package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDownloadFileWithResume(t *testing.T) {
	payload := []byte("abcdefghijklmnopqrstuvwxyz")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Agent-UUID"); got != "u1" {
			t.Fatalf("missing uuid header")
		}
		if got := r.Header.Get("X-Agent-Secret"); got != "s1" {
			t.Fatalf("missing secret header")
		}

		rng := r.Header.Get("Range")
		if rng == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload)
			return
		}

		parts := strings.Split(strings.TrimPrefix(rng, "bytes="), "-")
		start, err := strconv.Atoi(parts[0])
		if err != nil || start < 0 || start > len(payload) {
			t.Fatalf("bad range: %s", rng)
		}
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(payload[start:])
	}))
	defer srv.Close()

	tmp := t.TempDir()
	dest := filepath.Join(tmp, "app.bin")
	if err := os.WriteFile(dest, payload[:10], 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if _, err := DownloadFile(context.Background(), srv.URL, dest, 1024, "u1", "s1"); err != nil {
		t.Fatalf("download: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("unexpected content: %q", string(got))
	}
}
