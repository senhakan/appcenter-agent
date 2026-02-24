//go:build windows

package installer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func installMSI(ctx context.Context, filePath, args string) (int, error) {
	cmdArgs := []string{"/i", filePath}
	if strings.TrimSpace(args) != "" {
		cmdArgs = append(cmdArgs, strings.Fields(args)...)
	}
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("appcenter-msi-%d.log", time.Now().UnixNano()))
	cmdArgs = append(cmdArgs, "/L*v", logPath)

	cmd := exec.CommandContext(ctx, "msiexec", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		code := exitErr.ExitCode()
		// Common "success but reboot required" codes:
		// 3010 = ERROR_SUCCESS_REBOOT_REQUIRED
		// 1641 = ERROR_SUCCESS_REBOOT_INITIATED
		if code == 3010 || code == 1641 {
			return code, nil
		}
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			detail = summarizeMSILog(logPath)
		}
		if detail == "" {
			detail = fmt.Sprintf("exit code %d", code)
		}
		return code, fmt.Errorf("msi install failed: %s", detail)
	}
	return -1, err
}

func summarizeMSILog(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lastProductLine string
	var lastErrorLine string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.Contains(line, "Product:") && strings.Contains(line, "--") {
			lastProductLine = line
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			lastErrorLine = line
		}
	}
	if lastProductLine != "" {
		return lastProductLine
	}
	return lastErrorLine
}
