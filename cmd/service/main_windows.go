//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/windows/svc"
)

func main() {
	isService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to detect service mode: %v\n", err)
		os.Exit(1)
	}

	if isService {
		if err := svc.Run(serviceName, &appCenterService{}); err != nil {
			os.Exit(1)
		}
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := runAgent(ctx, resolveConfigPath()); err != nil {
		fmt.Fprintf(os.Stderr, "service error: %v\n", err)
		os.Exit(1)
	}
}
