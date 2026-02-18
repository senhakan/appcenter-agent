//go:build !windows

package system

// collectHostExtras returns empty extras on non-Windows platforms.
// Hardware introspection is only implemented for Windows.
func collectHostExtras() hostExtras {
	return hostExtras{}
}
