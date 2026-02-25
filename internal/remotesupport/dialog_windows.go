//go:build windows

package remotesupport

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	idYes               = 6
	approvalDialogTitle = "AppCenter - Uzak Destek Istegi"
)

var (
	approvalMu      sync.Mutex
	approvalProc    *os.Process
	approvalOutFile string
)

// ShowApprovalDialogFromService shows a cancelable user approval dialog from service context.
// It launches a popup process in active user session and waits for response file.
func ShowApprovalDialogFromService(adminName, reason string, timeoutSec int) (bool, error) {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if strings.TrimSpace(adminName) == "" {
		adminName = "Yonetici"
	}
	if strings.TrimSpace(reason) == "" {
		reason = "Belirtilmedi"
	}

	_ = CloseApprovalDialogFromService()

	adminName = psSingleQuote(adminName)
	reason = psSingleQuote(reason)
	title := psSingleQuote(approvalDialogTitle)

	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("appcenter_rs_approval_%d.txt", time.Now().UnixNano()))
	outPath = strings.ReplaceAll(outPath, "\\", "\\\\")

	// WScript.Shell Popup return codes:
	// 6 = Yes, 7 = No, -1 = timeout
	ps := "$ws=New-Object -ComObject WScript.Shell; " +
		"$nl=[Environment]::NewLine; " +
		"$msg='" + adminName + " asagidaki sebep ile ekraniniza baglanmak istiyor.' + $nl + $nl + '" + reason + "' + $nl + $nl + 'Onay veriyor musunuz?'; " +
		"$r=$ws.Popup($msg," + strconv.Itoa(timeoutSec) + ",'" + title + "',0x24); " +
		"Set-Content -Path '" + outPath + "' -Value $r -Encoding ascii -Force"

	psExe := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	proc, err := StartProcessInActiveUserSession(psExe, []string{
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle",
		"Hidden",
		"-ExecutionPolicy",
		"Bypass",
		"-Command",
		ps,
	})
	if err != nil {
		return false, fmt.Errorf("start approval popup: %w", err)
	}

	approvalMu.Lock()
	approvalProc = proc
	approvalOutFile = outPath
	approvalMu.Unlock()

	deadline := time.Now().Add(time.Duration(timeoutSec+2) * time.Second)
	for time.Now().Before(deadline) {
		if code, ok := readApprovalCode(outPath); ok {
			_ = CloseApprovalDialogFromService()
			return code == idYes, nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	_ = CloseApprovalDialogFromService()
	return false, nil
}

// CloseApprovalDialogFromService force-closes the active approval popup if present.
func CloseApprovalDialogFromService() error {
	approvalMu.Lock()
	proc := approvalProc
	out := approvalOutFile
	approvalProc = nil
	approvalOutFile = ""
	approvalMu.Unlock()

	if proc != nil {
		_ = proc.Kill()
		_, _ = proc.Wait()
	}
	if out != "" {
		_ = os.Remove(out)
	}
	return nil
}

func readApprovalCode(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func psSingleQuote(s string) string {
	// PowerShell single-quoted literal escaping.
	return strings.ReplaceAll(s, "'", "''")
}
