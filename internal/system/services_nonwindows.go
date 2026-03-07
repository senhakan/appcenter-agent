//go:build !windows

package system

func CollectServices() ([]ServiceInfo, error) {
	return []ServiceInfo{}, nil
}
