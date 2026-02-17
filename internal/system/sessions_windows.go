//go:build windows

package system

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// LoggedInSession represents a currently logged-in user session on the machine.
// SessionType is expected to be "local" or "rdp".
type LoggedInSession struct {
	Username    string `json:"username"`
	SessionType string `json:"session_type"`
	LogonID     string `json:"logon_id,omitempty"`
}

type wtsConnectStateClass uint32

const (
	wtsActive wtsConnectStateClass = 0
)

type wtsSessionInfoW struct {
	SessionID       uint32
	pWinStationName *uint16
	State           wtsConnectStateClass
}

const (
	wtsUserName           = 5
	wtsDomainName         = 7
	wtsClientProtocolType = 16
)

var (
	wtsapi32   = windows.NewLazySystemDLL("wtsapi32.dll")
	procEnum   = wtsapi32.NewProc("WTSEnumerateSessionsW")
	procFree   = wtsapi32.NewProc("WTSFreeMemory")
	procQuery  = wtsapi32.NewProc("WTSQuerySessionInformationW")
)

func GetLoggedInSessions() []LoggedInSession {
	// Enumerate terminal sessions from service context reliably using WTS APIs.
	var pSessions uintptr
	var count uint32

	// WTS_CURRENT_SERVER_HANDLE = 0
	r1, _, _ := procEnum.Call(
		0,
		0,
		1,
		uintptr(unsafe.Pointer(&pSessions)),
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 || pSessions == 0 || count == 0 {
		return nil
	}
	defer procFree.Call(pSessions)

	items := make([]LoggedInSession, 0, 4)
	seen := make(map[string]struct{}, 4)

	sz := unsafe.Sizeof(wtsSessionInfoW{})
	for i := uint32(0); i < count; i++ {
		sess := (*wtsSessionInfoW)(unsafe.Pointer(pSessions + uintptr(i)*sz))
		if sess == nil || sess.State != wtsActive {
			continue
		}

		user := wtsQueryString(sess.SessionID, wtsUserName)
		user = strings.TrimSpace(user)
		if user == "" {
			continue
		}
		domain := strings.TrimSpace(wtsQueryString(sess.SessionID, wtsDomainName))
		full := user
		if domain != "" {
			full = domain + `\` + user
		}
		if _, ok := seen[full]; ok {
			continue
		}
		seen[full] = struct{}{}

		proto := wtsQueryUint16(sess.SessionID, wtsClientProtocolType)
		sType := "local"
		if proto == 2 {
			sType = "rdp"
		}

		items = append(items, LoggedInSession{
			Username:    full,
			SessionType: sType,
			LogonID:     fmt.Sprintf("%d", sess.SessionID),
		})
	}

	return items
}

func wtsQueryString(sessionID uint32, infoClass uint32) string {
	var pBuf uintptr
	var bytes uint32
	r1, _, _ := procQuery.Call(
		0,
		uintptr(sessionID),
		uintptr(infoClass),
		uintptr(unsafe.Pointer(&pBuf)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 || pBuf == 0 || bytes == 0 {
		return ""
	}
	defer procFree.Call(pBuf)

	return windows.UTF16PtrToString((*uint16)(unsafe.Pointer(pBuf)))
}

func wtsQueryUint16(sessionID uint32, infoClass uint32) uint16 {
	var pBuf uintptr
	var bytes uint32
	r1, _, _ := procQuery.Call(
		0,
		uintptr(sessionID),
		uintptr(infoClass),
		uintptr(unsafe.Pointer(&pBuf)),
		uintptr(unsafe.Pointer(&bytes)),
	)
	if r1 == 0 || pBuf == 0 || bytes < 2 {
		return 0
	}
	defer procFree.Call(pBuf)

	return *(*uint16)(unsafe.Pointer(pBuf))
}

