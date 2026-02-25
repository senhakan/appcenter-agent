//go:build windows

package remotesupport

import (
	"fmt"
	"log"
	"sort"
	"syscall"
	"unsafe"
)

var (
	wtsapi32                         = syscall.NewLazyDLL("wtsapi32.dll")
	procWTSSendMessageW              = wtsapi32.NewProc("WTSSendMessageW")
	procWTSEnumerateSessionsW        = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSFreeMemory                = wtsapi32.NewProc("WTSFreeMemory")
	kernel32                         = syscall.NewLazyDLL("kernel32.dll")
	procWTSGetActiveConsoleSessionID = kernel32.NewProc("WTSGetActiveConsoleSessionId")
)

const (
	wtsCurrentServerHandle = 0
	mbYesNo                = 0x00000004
	mbIconQuestion         = 0x00000020
	idYes                  = 6
	wtsActive              = 0
)

type wtsSessionInfo struct {
	SessionID       uint32
	pWinStationName *uint16
	State           uint32
}

func candidateSessionIDs() []uint32 {
	ids := map[uint32]struct{}{}

	var pp uintptr
	var count uint32
	ret, _, _ := procWTSEnumerateSessionsW.Call(
		wtsCurrentServerHandle,
		0,
		1,
		uintptr(unsafe.Pointer(&pp)),
		uintptr(unsafe.Pointer(&count)),
	)
	if ret != 0 && pp != 0 && count > 0 {
		defer procWTSFreeMemory.Call(pp)
		size := unsafe.Sizeof(wtsSessionInfo{})
		for i := uint32(0); i < count; i++ {
			p := (*wtsSessionInfo)(unsafe.Pointer(pp + uintptr(i)*size))
			if p.State == wtsActive {
				ids[p.SessionID] = struct{}{}
			}
		}
	}

	consoleID, _, _ := procWTSGetActiveConsoleSessionID.Call()
	if consoleID != 0xFFFFFFFF {
		ids[uint32(consoleID)] = struct{}{}
	}

	out := make([]uint32, 0, len(ids))
	for sid := range ids {
		out = append(out, sid)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func orderedSessionIDs() []uint32 {
	all := candidateSessionIDs()
	if len(all) <= 1 {
		return all
	}
	consoleID, _, _ := procWTSGetActiveConsoleSessionID.Call()
	console := uint32(0xFFFFFFFF)
	if consoleID != 0xFFFFFFFF {
		console = uint32(consoleID)
	}

	out := make([]uint32, 0, len(all))
	if console != 0xFFFFFFFF {
		for _, sid := range all {
			if sid == console {
				out = append(out, sid)
				break
			}
		}
	}
	for _, sid := range all {
		if sid != console {
			out = append(out, sid)
		}
	}
	return out
}

// ShowApprovalDialogFromService shows a user approval dialog from service context.
func ShowApprovalDialogFromService(adminName, reason string, timeoutSec int) (bool, error) {
	sessions := orderedSessionIDs()
	if len(sessions) == 0 {
		return false, fmt.Errorf("no active user session")
	}
	log.Printf("remote support dialog: candidate sessions=%v timeout=%ds", sessions, timeoutSec)

	if reason == "" {
		reason = "Belirtilmedi"
	}
	title, _ := syscall.UTF16FromString("AppCenter - Uzak Destek Istegi")
	message, _ := syscall.UTF16FromString(fmt.Sprintf(
		"%s asagidaki sebep ile ekraniniza baglanmak istiyor.\n\n%s\n\nOnay veriyor musunuz?",
		adminName,
		reason,
	))

	var response uint32
	var lastErr error
	for _, sessionID := range sessions {
		response = 0
		ret, _, err := procWTSSendMessageW.Call(
			wtsCurrentServerHandle,
			uintptr(sessionID),
			uintptr(unsafe.Pointer(&title[0])),
			uintptr(len(title)*2),
			uintptr(unsafe.Pointer(&message[0])),
			uintptr(len(message)*2),
			uintptr(mbYesNo|mbIconQuestion),
			uintptr(timeoutSec),
			uintptr(unsafe.Pointer(&response)),
			uintptr(1), // wait for response
		)
		if ret != 0 {
			log.Printf("remote support dialog: session=%d response=%d", sessionID, response)
			return response == idYes, nil
		}
		log.Printf("remote support dialog: session=%d send failed: %v", sessionID, err)
		lastErr = err
	}
	if lastErr != nil {
		return false, fmt.Errorf("WTSSendMessage failed for all sessions: %v", lastErr)
	}
	return false, fmt.Errorf("WTSSendMessage failed for all sessions")
}
