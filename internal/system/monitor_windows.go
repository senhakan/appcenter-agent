//go:build windows

package system

import "golang.org/x/sys/windows"

var (
	modUser32            = windows.NewLazySystemDLL("user32.dll")
	procGetSystemMetrics = modUser32.NewProc("GetSystemMetrics")
)

const smCMonitors = 80

// MonitorCount returns the number of display monitors on Windows.
func MonitorCount() int {
	ret, _, _ := procGetSystemMetrics.Call(uintptr(smCMonitors))
	n := int(ret)
	if n <= 0 {
		return 1
	}
	return n
}

