//go:build windows

package remotesupport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	userenvDLL                  = windows.NewLazySystemDLL("userenv.dll")
	procCreateEnvironmentBlock  = userenvDLL.NewProc("CreateEnvironmentBlock")
	procDestroyEnvironmentBlock = userenvDLL.NewProc("DestroyEnvironmentBlock")

	wtsapi32DLL           = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQueryUserToken = wtsapi32DLL.NewProc("WTSQueryUserToken")
)

func startHelperProcess(exePath string, args []string) (*os.Process, error) {
	sessionID := activeSessionForUserProcess()

	userToken, err := primaryTokenFromExplorer(sessionID)
	if err != nil {
		userToken, err = primaryTokenFromSession(sessionID)
		if err != nil {
			return nil, fmt.Errorf("resolve interactive user token (session=%d): %w", sessionID, err)
		}
	}
	userToken = preferElevatedLinkedToken(userToken)
	defer userToken.Close()

	cmdLine, err := windows.UTF16PtrFromString(buildWindowsCmdline(exePath, args))
	if err != nil {
		return nil, fmt.Errorf("build command line: %w", err)
	}
	appName, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return nil, fmt.Errorf("app path utf16: %w", err)
	}
	workDir, err := windows.UTF16PtrFromString(filepath.Dir(exePath))
	if err != nil {
		return nil, fmt.Errorf("workdir utf16: %w", err)
	}
	desktop, err := windows.UTF16PtrFromString("winsta0\\default")
	if err != nil {
		return nil, fmt.Errorf("desktop utf16: %w", err)
	}

	var env uintptr
	if r1, _, callErr := procCreateEnvironmentBlock.Call(uintptr(unsafe.Pointer(&env)), uintptr(userToken), 0); r1 == 0 {
		return nil, fmt.Errorf("CreateEnvironmentBlock: %w", callErr)
	}
	defer procDestroyEnvironmentBlock.Call(env)

	si := new(windows.StartupInfo)
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.Desktop = desktop
	var pi windows.ProcessInformation

	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT)
	if err := windows.CreateProcessAsUser(
		userToken,
		appName,
		cmdLine,
		nil,
		nil,
		false,
		flags,
		(*uint16)(unsafe.Pointer(env)),
		workDir,
		si,
		&pi,
	); err != nil {
		return nil, fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)

	return os.FindProcess(int(pi.ProcessId))
}

func activeSessionForUserProcess() uint32 {
	sessions := orderedSessionIDs()
	if len(sessions) > 0 {
		return sessions[0]
	}
	consoleID, _, _ := procWTSGetActiveConsoleSessionID.Call()
	if consoleID != 0xFFFFFFFF {
		return uint32(consoleID)
	}
	return 0
}

func primaryTokenFromExplorer(sessionID uint32) (windows.Token, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(snap)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if err := windows.Process32First(snap, &pe); err != nil {
		return 0, err
	}
	for {
		name := windows.UTF16ToString(pe.ExeFile[:])
		if strings.EqualFold(name, "explorer.exe") {
			var sid uint32
			if err := windows.ProcessIdToSessionId(pe.ProcessID, &sid); err == nil && sid == sessionID {
				return duplicatePrimaryTokenFromPID(pe.ProcessID)
			}
		}
		if err := windows.Process32Next(snap, &pe); err != nil {
			if err == syscall.ERROR_NO_MORE_FILES {
				break
			}
			return 0, err
		}
	}
	return 0, fmt.Errorf("explorer.exe not found in session %d", sessionID)
}

func primaryTokenFromSession(sessionID uint32) (windows.Token, error) {
	var impToken windows.Token
	r1, _, callErr := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&impToken)))
	if r1 == 0 {
		return 0, callErr
	}
	defer impToken.Close()
	return duplicatePrimaryToken(impToken)
}

func duplicatePrimaryTokenFromPID(pid uint32) (windows.Token, error) {
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(proc)

	var token windows.Token
	if err := windows.OpenProcessToken(proc, windows.TOKEN_QUERY|windows.TOKEN_DUPLICATE, &token); err != nil {
		return 0, err
	}
	defer token.Close()

	return duplicatePrimaryToken(token)
}

func duplicatePrimaryToken(src windows.Token) (windows.Token, error) {
	var primary windows.Token
	if err := windows.DuplicateTokenEx(
		src,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&primary,
	); err != nil {
		return 0, err
	}
	return primary, nil
}

func preferElevatedLinkedToken(base windows.Token) windows.Token {
	linked, err := base.GetLinkedToken()
	if err != nil {
		return base
	}
	// For UAC-admin users, base token is usually filtered (medium) and linked token is elevated.
	if linked.IsElevated() {
		_ = base.Close()
		return linked
	}
	_ = linked.Close()
	return base
}

func buildWindowsCmdline(exePath string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, syscall.EscapeArg(exePath))
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}
