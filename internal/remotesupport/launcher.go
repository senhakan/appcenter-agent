package remotesupport

import "os"

// StartProcessInActiveUserSession launches a process into the current interactive
// user session from the service context.
func StartProcessInActiveUserSession(exePath string, args []string) (*os.Process, error) {
	return startHelperProcess(exePath, args)
}
