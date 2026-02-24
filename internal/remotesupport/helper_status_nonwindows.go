//go:build !windows

package remotesupport

func helperProcessStatus() (bool, int) {
	return false, 0
}
