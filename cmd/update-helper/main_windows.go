//go:build windows

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"appcenter-agent/internal/config"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func main() {
	var (
		serviceName   = flag.String("service-name", "AppCenterAgent", "Windows service name")
		targetExe     = flag.String("target-exe", "", "Path to current service executable")
		newExe        = flag.String("new-exe", "", "Path to staged update executable")
		metaPath      = flag.String("meta", "", "Path to pending_update.json")
		configPath    = flag.String("config", "", "Path to agent config.yaml")
		targetVersion = flag.String("target-version", "", "Version string to write into config")
		logPath       = flag.String("log", `C:\ProgramData\AppCenter\logs\update-helper.log`, "Log file path")
		timeoutSec    = flag.Int("timeout-sec", 120, "Timeout seconds for stop/start operations")
	)
	flag.Parse()

	logger := log.New(os.Stdout, "update-helper ", log.LstdFlags)
	if *logPath != "" {
		if err := os.MkdirAll(filepath.Dir(*logPath), 0o755); err == nil {
			if f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
				defer f.Close()
				logger.SetOutput(io.MultiWriter(os.Stdout, f))
			}
		}
	}

	if *targetExe == "" || *newExe == "" || *configPath == "" || *targetVersion == "" {
		logger.Fatalf("missing required args: target-exe/new-exe/config/target-version")
	}

	// Stop service
	m, err := mgr.Connect()
	if err != nil {
		logger.Fatalf("mgr connect: %v", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(*serviceName)
	if err != nil {
		logger.Fatalf("open service %q: %v", *serviceName, err)
	}
	defer s.Close()

	logger.Printf("stopping service %q...", *serviceName)
	_, _ = s.Control(svc.Stop)
	if err := waitForState(s, svc.Stopped, time.Duration(*timeoutSec)*time.Second); err != nil {
		logger.Fatalf("wait stop: %v", err)
	}

	backup := *targetExe + ".bak." + time.Now().UTC().Format("20060102T150405Z")
	logger.Printf("backup current exe: %s", backup)
	if err := copyFile(*targetExe, backup); err != nil {
		logger.Fatalf("backup copy: %v", err)
	}

	// Replace binary
	logger.Printf("replacing exe: %s -> %s", *newExe, *targetExe)
	if err := replaceFile(*newExe, *targetExe); err != nil {
		_ = replaceFile(backup, *targetExe)
		logger.Fatalf("replace failed: %v", err)
	}

	// Update config version to prevent repeated staging.
	if err := updateConfigVersion(*configPath, *targetVersion); err != nil {
		logger.Printf("warning: config version update failed: %v", err)
	}

	// Cleanup metadata; ignore errors.
	if *metaPath != "" {
		_ = os.Remove(*metaPath)
	}

	logger.Printf("starting service %q...", *serviceName)
	if err := s.Start(); err != nil {
		// rollback
		_ = replaceFile(backup, *targetExe)
		_ = s.Start()
		logger.Fatalf("start failed: %v", err)
	}
	if err := waitForState(s, svc.Running, time.Duration(*timeoutSec)*time.Second); err != nil {
		logger.Printf("warning: service not running after start: %v", err)
	}

	logger.Printf("update apply completed: version=%s", *targetVersion)
}

func waitForState(s *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == want {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for state=%v", want)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func replaceFile(src, dst string) error {
	// Copy + rename keeps src intact if copy fails. We'll remove src at the end.
	tmp := dst + ".new"
	if err := copyFile(src, tmp); err != nil {
		return err
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func updateConfigVersion(path string, version string) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	cfg.Agent.Version = version
	return config.Save(path, cfg)
}
