//go:build windows

package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func isProcessRunningByImage(imageName string) bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return false
	}
	for {
		name := windows.UTF16ToString(pe.ExeFile[:])
		if strings.EqualFold(name, imageName) {
			return true
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			break
		}
	}
	return false
}
