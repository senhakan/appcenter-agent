//go:build !windows

package installer

import (
	"context"
	"fmt"
)

func installMSI(_ context.Context, _ string, _ string) (int, error) {
	return -1, fmt.Errorf("msi installation is only supported on windows")
}
