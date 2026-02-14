//go:build windows

package installer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func installMSI(ctx context.Context, filePath, args string) (int, error) {
	cmdArgs := []string{"/i", filePath}
	if strings.TrimSpace(args) != "" {
		cmdArgs = append(cmdArgs, strings.Fields(args)...)
	}

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
		return code, fmt.Errorf("msi install failed: %s", strings.TrimSpace(string(out)))
	}
	return -1, err
}
