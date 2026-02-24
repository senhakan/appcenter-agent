//go:build !windows

package tray

import "fmt"

func Run() error {
	return fmt.Errorf("systray mode is only supported on windows")
}

func OpenStoreWindowStandalone() error {
	return fmt.Errorf("store window is only supported on windows")
}

func OpenStoreNativeUIStandalone() error {
	return fmt.Errorf("store window is only supported on windows")
}

func OpenStoreUI() error {
	return fmt.Errorf("store window is only supported on windows")
}
