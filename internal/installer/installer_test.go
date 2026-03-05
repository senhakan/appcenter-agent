package installer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallEXE(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("linux script-based test")
	}

	tmp := t.TempDir()
	installerPath := filepath.Join(tmp, "ok.exe")
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(installerPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write installer: %v", err)
	}

	exitCode, err := Install(installerPath, "", 5)
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}

func TestInstallUnsupportedType(t *testing.T) {
	_, err := Install("/tmp/file.zip", "", 5)
	if err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestInstallPS1NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows behavior")
	}
	_, err := Install("/tmp/install.ps1", "", 5)
	if err == nil {
		t.Fatal("expected ps1 install error")
	}
	if !strings.Contains(err.Error(), "only supported on windows") {
		t.Fatalf("unexpected error: %v", err)
	}
}
