//go:build windows

package system

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modKernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modKernel32.NewProc("GlobalMemoryStatusEx")
)

// memoryStatusEx mirrors MEMORYSTATUSEX from the Windows API.
type memoryStatusEx struct {
	DwLength                uint32
	DwMemoryLoad            uint32
	UllTotalPhys            uint64
	UllAvailPhys            uint64
	UllTotalPageFile        uint64
	UllAvailPageFile        uint64
	UllTotalVirtual         uint64
	UllAvailVirtual         uint64
	UllAvailExtendedVirtual uint64
}

func collectHostExtras() hostExtras {
	var extras hostExtras

	// CPU model from registry — fast, no external process needed.
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if name, _, err := k.GetStringValue("ProcessorNameString"); err == nil {
			extras.CPUModel = strings.TrimSpace(name)
		}
	}

	// Total physical RAM via GlobalMemoryStatusEx.
	var mem memoryStatusEx
	mem.DwLength = uint32(unsafe.Sizeof(mem))
	if ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&mem))); ret != 0 {
		extras.RAMGB = int(mem.UllTotalPhys / (1024 * 1024 * 1024))
	}

	// Free disk space on the system drive (C:\).
	drivePath, err := windows.UTF16PtrFromString(`C:\`)
	if err == nil {
		var freeAvail, totalBytes, totalFree uint64
		if err := windows.GetDiskFreeSpaceEx(drivePath, &freeAvail, &totalBytes, &totalFree); err == nil {
			extras.DiskFreeGB = int(totalFree / (1024 * 1024 * 1024))
		}
	}

	return extras
}
