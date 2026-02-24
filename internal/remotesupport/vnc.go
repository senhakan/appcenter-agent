package remotesupport

import (
	"crypto/des"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	vncHelperName = "acremote-helper.exe"
	vncConfigName = "acremote.ini"
)

// VNCServer controls the helper process lifecycle.
type VNCServer struct {
	exePath    string
	configPath string
	process    *os.Process
	logger     *log.Logger
	retryStop  chan struct{}
	retryWG    sync.WaitGroup
}

func NewVNCServer(logger *log.Logger) *VNCServer {
	exeDir := exeDirectory()
	cfg := filepath.Join(dataDirectory(), vncConfigName)
	if runtime.GOOS == "windows" {
		// UltraVNC (WinVNC) reads default config from Windows directory.
		cfg = `C:\Windows\ultravnc.ini`
	}
	return &VNCServer{
		exePath:    filepath.Join(exeDir, vncHelperName),
		configPath: cfg,
		logger:     logger,
	}
}

func (v *VNCServer) Available() bool {
	_, err := os.Stat(v.exePath)
	return err == nil
}

func (v *VNCServer) Start(password, guacdHost string, guacdPort int) error {
	if !v.Available() {
		return fmt.Errorf("vnc helper not found: %s", v.exePath)
	}
	if err := v.writeConfig(password); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := v.writeRegistryPassword(password); err != nil {
			v.logger.Printf("remote support: registry password write warning: %v", err)
		}
	}

	cmd := exec.Command(v.exePath, "-run")
	cmd.Dir = filepath.Dir(v.exePath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start helper: %w", err)
	}
	v.process = cmd.Process
	v.logger.Printf("remote support: helper started pid=%d", v.process.Pid)

	// Reverse-connect is optional. In direct VNC mode we only expose local 5900.
	if guacdHost != "" && guacdPort > 0 {
		// Keep retrying reverse-connect while session is active. This prevents
		// race conditions where guacd listener starts after the first connect attempt.
		target := fmt.Sprintf("%s::%d", guacdHost, guacdPort)
		v.startRetryLoop(target)
	}

	return nil
}

func (v *VNCServer) Stop() {
	v.stopRetryLoop()
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
	// Defensive cleanup: if helper remains in another session/process tree, force-kill by image name.
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/F", "/IM", vncHelperName).Run()
	}
	_ = os.Remove(v.configPath)
}

func (v *VNCServer) Status() (bool, int) {
	return helperProcessStatus()
}

func (v *VNCServer) startRetryLoop(target string) {
	v.stopRetryLoop()
	stopCh := make(chan struct{})
	v.retryStop = stopCh
	v.retryWG.Add(1)
	go func() {
		defer v.retryWG.Done()
		time.Sleep(2 * time.Second)
		v.tryReverseConnect(target)
		ticker := time.NewTicker(8 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				v.tryReverseConnect(target)
			}
		}
	}()
}

func (v *VNCServer) stopRetryLoop() {
	if v.retryStop != nil {
		close(v.retryStop)
		v.retryStop = nil
		v.retryWG.Wait()
	}
}

func (v *VNCServer) tryReverseConnect(target string) {
	connectCmd := exec.Command(v.exePath, "-connect", target)
	if err := connectCmd.Run(); err != nil {
		v.logger.Printf("remote support: reverse connect warning: %v", err)
	}
}

func (v *VNCServer) writeConfig(password string) error {
	if err := os.MkdirAll(filepath.Dir(v.configPath), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	encPass, err := encryptUltraVNCPassword(password)
	if err != nil {
		return fmt.Errorf("encrypt vnc password: %w", err)
	}
	content := fmt.Sprintf(`[UltraVNC]
PortNumber=5900
AllowLoopback=0
LoopbackOnly=0
passwd=%s
passwd2=%s
AuthRequired=1
ConnectPriority=0
InputsEnabled=1
DebugMode=0
DebugLevel=0
UseRegistry=0
UseDDEngine=1
UseMirrorDriver=0
FileTransferEnabled=0
FTUserImpersonation=0
`, encPass, encPass)
	if err := os.WriteFile(v.configPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func encryptUltraVNCPassword(password string) (string, error) {
	// UltraVNC stores password as DES-ECB encrypted 8-byte block plus "00" suffix.
	key := []byte{0xE8, 0x4A, 0xD6, 0x60, 0xC4, 0x72, 0x1A, 0xE0}
	block, err := des.NewCipher(key)
	if err != nil {
		return "", err
	}
	src := make([]byte, 8)
	for i := 0; i < len(password) && i < 8; i++ {
		src[i] = password[i]
	}
	dst := make([]byte, 8)
	block.Encrypt(dst, src)
	return strings.ToUpper(hex.EncodeToString(dst)) + "00", nil
}

func encryptUltraVNCPasswordRaw(password string) ([]byte, error) {
	key := []byte{0xE8, 0x4A, 0xD6, 0x60, 0xC4, 0x72, 0x1A, 0xE0}
	block, err := des.NewCipher(key)
	if err != nil {
		return nil, err
	}
	src := make([]byte, 8)
	for i := 0; i < len(password) && i < 8; i++ {
		src[i] = password[i]
	}
	dst := make([]byte, 8)
	block.Encrypt(dst, src)
	return dst, nil
}

func (v *VNCServer) writeRegistryPassword(password string) error {
	raw, err := encryptUltraVNCPasswordRaw(password)
	if err != nil {
		return err
	}
	hexRaw := strings.ToUpper(hex.EncodeToString(raw))
	cmd1 := exec.Command("reg", "add", `HKLM\SOFTWARE\ORL\WinVNC3`, "/v", "Password", "/t", "REG_BINARY", "/d", hexRaw, "/f")
	if err := cmd1.Run(); err != nil {
		return err
	}
	cmd2 := exec.Command("reg", "add", `HKLM\SOFTWARE\ORL\WinVNC3`, "/v", "PasswordViewOnly", "/t", "REG_BINARY", "/d", hexRaw, "/f")
	if err := cmd2.Run(); err != nil {
		return err
	}
	return nil
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
