//go:build !windows

package system

// MonitorCount returns a safe default on non-Windows platforms.
func MonitorCount() int {
	return 1
}

