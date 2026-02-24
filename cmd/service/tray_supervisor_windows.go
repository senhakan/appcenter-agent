//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"appcenter-agent/internal/remotesupport"
)

const trayExeName = "appcenter-tray.exe"

type traySupervisor struct {
	logger  *log.Logger
	exePath string
}

func newTraySupervisor(serviceExe string, logger *log.Logger) *traySupervisor {
	return &traySupervisor{
		logger:  logger,
		exePath: filepath.Join(filepath.Dir(serviceExe), trayExeName),
	}
}

func (t *traySupervisor) SetEnabled(enabled bool) {
	if enabled {
		t.ensureRunning()
		return
	}
	t.stopAll()
}

func (t *traySupervisor) ensureRunning() {
	if _, err := os.Stat(t.exePath); err != nil {
		t.logger.Printf("tray supervisor: %s not found: %v", t.exePath, err)
		return
	}
	if running := isProcessRunningByImage(trayExeName); running {
		return
	}
	proc, err := remotesupport.StartProcessInActiveUserSession(t.exePath, nil)
	if err != nil {
		t.logger.Printf("tray supervisor: start failed: %v", err)
		return
	}
	t.logger.Printf("tray supervisor: started pid=%d", proc.Pid)
}

func (t *traySupervisor) stopAll() {
	cmd := exec.Command("taskkill", "/F", "/IM", trayExeName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.ToLower(string(out))
		if strings.Contains(msg, "not found") || strings.Contains(msg, "no running instance") {
			return
		}
		t.logger.Printf("tray supervisor: stop failed: %v (%s)", err, strings.TrimSpace(string(out)))
		return
	}
	time.Sleep(150 * time.Millisecond)
}
