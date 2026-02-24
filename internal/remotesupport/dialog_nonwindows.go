//go:build !windows

package remotesupport

import "errors"

func ShowApprovalDialogFromService(adminName, reason string, timeoutSec int) (bool, error) {
	_ = adminName
	_ = reason
	_ = timeoutSec
	return false, errors.New("remote support dialog is only available on windows")
}
