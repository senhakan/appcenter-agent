//go:build !windows

package announcement

import "log"

func ShowMessageBox(title, message, priority string) {
	log.Printf("announcement: messagebox noop (non-windows) title=%q priority=%q message=%q", title, priority, message)
}
