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

type approvalResult struct {
	approved     bool
	monitorCount int
}

// ShowApprovalDialogFromService shows a cancelable user approval dialog from service context.
// It launches a popup process in active user session and waits for response file.
func ShowApprovalDialogFromService(adminName, reason string, timeoutSec int) (bool, int, error) {
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
		"$mc=1; " +
		"try { Add-Type -AssemblyName System.Windows.Forms -ErrorAction Stop; $mc=[System.Windows.Forms.Screen]::AllScreens.Count } catch { $mc=1 }; " +
		"if ([int]$mc -le 0) { $mc=1 }; " +
		"Set-Content -Path '" + outPath + "' -Value ($r.ToString() + '|' + $mc.ToString()) -Encoding ascii -Force"

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
		return false, 1, fmt.Errorf("start approval popup: %w", err)
	}

	approvalMu.Lock()
	approvalProc = proc
	approvalOutFile = outPath
	approvalMu.Unlock()

	deadline := time.Now().Add(time.Duration(timeoutSec+2) * time.Second)
	for time.Now().Before(deadline) {
		if result, ok := readApprovalResult(outPath); ok {
			_ = CloseApprovalDialogFromService()
			return result.approved, result.monitorCount, nil
		}
		time.Sleep(250 * time.Millisecond)
	}

	_ = CloseApprovalDialogFromService()
	return false, 1, nil
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

func readApprovalResult(path string) (approvalResult, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return approvalResult{}, false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return approvalResult{}, false
	}
	parts := strings.Split(s, "|")
	if len(parts) == 0 {
		return approvalResult{}, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return approvalResult{}, false
	}
	mon := 1
	if len(parts) >= 2 {
		if n, convErr := strconv.Atoi(strings.TrimSpace(parts[1])); convErr == nil && n > 0 {
			mon = n
		}
	}
	return approvalResult{
		approved:     code == idYes,
		monitorCount: mon,
	}, true
}

func psSingleQuote(s string) string {
	// PowerShell single-quoted literal escaping.
	return strings.ReplaceAll(s, "'", "''")
}
