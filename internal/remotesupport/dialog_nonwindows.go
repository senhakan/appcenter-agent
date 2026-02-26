//go:build !windows

package remotesupport

import "errors"

func ShowApprovalDialogFromService(adminName, reason string, timeoutSec int) (bool, int, error) {
	_ = adminName
	_ = reason
	_ = timeoutSec
	return false, 1, errors.New("remote support dialog is only available on windows")
}

func CloseApprovalDialogFromService() {}
