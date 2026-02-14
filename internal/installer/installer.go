package installer

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func Install(filePath, args string, timeoutSec int) (int, error) {
	if timeoutSec <= 0 {
		timeoutSec = 1800
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".msi":
		return installMSI(ctx, filePath, args)
	case ".exe":
		return installEXE(ctx, filePath, args)
	default:
		return -1, fmt.Errorf("unsupported installer type: %s", filepath.Ext(filePath))
	}
}
