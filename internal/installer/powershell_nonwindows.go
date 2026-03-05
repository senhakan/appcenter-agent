//go:build !windows

package installer

import (
	"context"
	"fmt"
)

func installPowerShell(_ context.Context, _ string, _ string) (int, error) {
	return -1, fmt.Errorf("powershell script installation is only supported on windows")
}
