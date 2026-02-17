//go:build !windows

package inventory

// ScanInstalledSoftware is a no-op on non-Windows platforms.
func ScanInstalledSoftware() []SoftwareItem {
	return nil
}
