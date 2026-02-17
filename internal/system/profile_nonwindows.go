//go:build !windows

package system

// CollectSystemProfile returns nil on non-Windows platforms.
func CollectSystemProfile() (*SystemProfile, error) {
	return nil, nil
}

