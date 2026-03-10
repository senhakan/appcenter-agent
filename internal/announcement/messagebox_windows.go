//go:build windows

package announcement

import (
	"log"
	"strings"
	"syscall"
	"unsafe"
)

const (
	mbOK          = 0x00000000
	mbIconError   = 0x00000010
	mbIconWarning = 0x00000030
	mbIconInfo    = 0x00000040
	mbTopMost     = 0x00040000

	wtsCurrentServerHandle = 0

	defaultAnnTitle = "Announcement"
)

var (
	wtsapi32           = syscall.NewLazyDLL("wtsapi32.dll")
	procWTSSendMessage = wtsapi32.NewProc("WTSSendMessageW")

	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procWTSGetActiveConsoleSessionId = kernel32.NewProc("WTSGetActiveConsoleSessionId")

	wtsapi32EnumSessions    = wtsapi32.NewProc("WTSEnumerateSessionsW")
	wtsapi32FreeMemory      = wtsapi32.NewProc("WTSFreeMemory")
)

const (
	wtsActive       = 0
	wtsConnected    = 1
)

// WTS_SESSION_INFO_W structure
type wtsSessionInfo struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

func ShowMessageBox(title, message, priority string) {
	if strings.TrimSpace(title) == "" {
		title = defaultAnnTitle
	}
	if strings.TrimSpace(message) == "" {
		message = " "
	}

	flags := uint32(mbOK | mbTopMost | iconFlags(priority))

	sessions := getActiveSessions()
	if len(sessions) == 0 {
		log.Printf("announcement: no active user sessions found, title=%q", title)
		return
	}

	shown := false
	for _, sessionID := range sessions {
		if sendMessageToSession(sessionID, title, message, flags, 0) {
			log.Printf("announcement: shown to session=%d title=%q", sessionID, title)
			shown = true
		}
	}
	if !shown {
		log.Printf("announcement: WTSSendMessage failed for all sessions, title=%q", title)
	}
}

func getActiveSessions() []uint32 {
	var pSessionInfo uintptr
	var count uint32

	ret, _, _ := wtsapi32EnumSessions.Call(
		wtsCurrentServerHandle,
		0, // reserved
		1, // version
		uintptr(unsafe.Pointer(&pSessionInfo)),
		uintptr(unsafe.Pointer(&count)),
	)
	if ret == 0 || count == 0 {
		// Fallback: try WTSGetActiveConsoleSessionId
		sid, _, _ := procWTSGetActiveConsoleSessionId.Call()
		if sid != 0xFFFFFFFF && sid != 0 {
			return []uint32{uint32(sid)}
		}
		return nil
	}
	defer wtsapi32FreeMemory.Call(pSessionInfo)

	var sessions []uint32
	infoSize := unsafe.Sizeof(wtsSessionInfo{})
	for i := uint32(0); i < count; i++ {
		info := (*wtsSessionInfo)(unsafe.Pointer(pSessionInfo + uintptr(i)*infoSize))
		// Only active/connected sessions, skip session 0 (services)
		if info.SessionID == 0 {
			continue
		}
		if info.State == wtsActive || info.State == wtsConnected {
			sessions = append(sessions, info.SessionID)
		}
	}

	if len(sessions) == 0 {
		// Fallback
		sid, _, _ := procWTSGetActiveConsoleSessionId.Call()
		if sid != 0xFFFFFFFF && sid != 0 {
			return []uint32{uint32(sid)}
		}
	}
	return sessions
}

func sendMessageToSession(sessionID uint32, title, message string, style uint32, timeout uint32) bool {
	titleUTF16, err := syscall.UTF16FromString(title)
	if err != nil {
		return false
	}
	messageUTF16, err := syscall.UTF16FromString(message)
	if err != nil {
		return false
	}

	titleLen := uint32(len(titleUTF16) * 2)   // byte count
	msgLen := uint32(len(messageUTF16) * 2)     // byte count

	var response uint32
	ret, _, callErr := procWTSSendMessage.Call(
		wtsCurrentServerHandle,
		uintptr(sessionID),
		uintptr(unsafe.Pointer(&titleUTF16[0])),
		uintptr(titleLen),
		uintptr(unsafe.Pointer(&messageUTF16[0])),
		uintptr(msgLen),
		uintptr(style),
		uintptr(timeout), // 0 = no timeout, wait for user
		uintptr(unsafe.Pointer(&response)),
		0, // bWait = FALSE (don't block)
	)
	if ret == 0 {
		log.Printf("announcement: WTSSendMessage failed session=%d err=%v", sessionID, callErr)
		return false
	}
	return true
}

func iconFlags(priority string) int {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "critical":
		return mbIconError
	case "important":
		return mbIconWarning
	case "normal":
		return mbIconInfo
	default:
		return mbIconInfo
	}
}
