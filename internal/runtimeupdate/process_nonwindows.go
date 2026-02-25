//go:build !windows

package runtimeupdate

func isProcessRunningByImage(_ string) bool {
	return false
}

func stopTrayProcess() {}
