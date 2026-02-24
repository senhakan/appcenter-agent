//go:build !windows

package remotesupport

import (
	"os"
	"os/exec"
	"path/filepath"
)

func startHelperProcess(exePath string, args []string) (*os.Process, error) {
	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}
