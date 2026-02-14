//go:build !windows

package tray

import "fmt"

func Run() error {
	return fmt.Errorf("systray mode is only supported on windows")
}
