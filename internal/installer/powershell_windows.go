//go:build windows

package installer

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func installPowerShell(ctx context.Context, filePath, args string) (int, error) {
	cmdArgs := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filePath}
	if strings.TrimSpace(args) != "" {
		cmdArgs = append(cmdArgs, strings.Fields(args)...)
	}

	cmd := exec.CommandContext(ctx, "powershell.exe", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), fmt.Errorf("powershell install failed: %s", strings.TrimSpace(string(out)))
	}
	return -1, err
}
