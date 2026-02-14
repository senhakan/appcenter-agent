package system

import (
	"net"
	"os"
	"runtime"
)

type HostInfo struct {
	Hostname   string
	IPAddress  string
	OSVersion  string
	CPUModel   string
	RAMGB      int
	DiskFreeGB int
}

func CollectHostInfo() HostInfo {
	hostname, _ := os.Hostname()
	return HostInfo{
		Hostname:   hostname,
		IPAddress:  firstIPv4(),
		OSVersion:  runtime.GOOS + "/" + runtime.GOARCH,
		CPUModel:   runtime.GOARCH,
		RAMGB:      0,
		DiskFreeGB: 0,
	}
}

func firstIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			if ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}
