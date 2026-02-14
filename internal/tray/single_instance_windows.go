//go:build windows

package tray

import (
	"errors"

	"golang.org/x/sys/windows"
)

// ensureSingleInstance prevents duplicate tray instances in the same user session.
func ensureSingleInstance() (func(), error) {
	name, err := windows.UTF16PtrFromString(`Local\AppCenterTray`)
	if err != nil {
		return nil, err
	}

	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, err
	}

	// If it already exists, we are a duplicate instance.
	if errors.Is(windows.GetLastError(), windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(h)
		return nil, errors.New("tray already running in this session")
	}

	return func() {
		_ = windows.CloseHandle(h)
	}, nil
}

