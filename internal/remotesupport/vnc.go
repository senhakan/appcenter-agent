package remotesupport

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const (
	vncHelperName = "rshelper.exe"
	vncConfigName = "acremote.ini"
)

// VNCServer controls the helper process lifecycle.
type VNCServer struct {
	exePath    string
	configPath string
	process    *os.Process
	logger     *log.Logger
}

func NewVNCServer(logger *log.Logger) *VNCServer {
	exeDir := exeDirectory()
	return &VNCServer{
		exePath:    filepath.Join(exeDir, vncHelperName),
		configPath: filepath.Join(dataDirectory(), vncConfigName),
		logger:     logger,
	}
}

func (v *VNCServer) Available() bool {
	_, err := os.Stat(v.exePath)
	return err == nil
}

func (v *VNCServer) Start(password string, port int) error {
	if !v.Available() {
		return fmt.Errorf("vnc helper not found: %s", v.exePath)
	}
	// Always ensure a clean process before re-starting.
	v.Stop()

	proc, err := startHelperProcess(v.exePath, []string{"-run", "-port", strconv.Itoa(port)})
	if err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	v.process = proc
	v.logger.Printf("remote support: helper started pid=%d port=%d", v.process.Pid, port)
	return nil
}

func (v *VNCServer) Stop() {
	if v.Available() {
		killCmd := exec.Command(v.exePath, "-kill")
		if err := killCmd.Run(); err != nil {
			v.logger.Printf("remote support: helper -kill failed: %v", err)
		}
	}
	if v.process != nil {
		_ = v.process.Kill()
		v.process = nil
	}
	// Defensive cleanup by image name.
	_ = exec.Command("taskkill", "/F", "/IM", vncHelperName).Run()
}

func (v *VNCServer) Status() (bool, int) {
	return helperProcessStatus()
}

func (v *VNCServer) WaitListening(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 700*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func exeDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func dataDirectory() string {
	pd := os.Getenv("PROGRAMDATA")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "AppCenter")
}
