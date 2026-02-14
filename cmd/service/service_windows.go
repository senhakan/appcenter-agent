//go:build windows

package main

import (
	"context"
	"errors"

	"golang.org/x/sys/windows/svc"

	"appcenter-agent/internal/updater"
)

type appCenterService struct{}

func (m *appCenterService) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var runErr error
	done := make(chan struct{})
	go func() {
		runErr = runAgent(ctx, resolveConfigPath())
		close(done)
	}()

	status <- svc.Status{State: svc.Running, Accepts: accepted}

	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				<-done
				if runErr != nil {
					return false, 1
				}
				return false, 0
			}
		case <-done:
			if runErr != nil && !errors.Is(runErr, updater.ErrUpdateRestart) {
				return false, 1
			}
			return false, 0
		}
	}
}
