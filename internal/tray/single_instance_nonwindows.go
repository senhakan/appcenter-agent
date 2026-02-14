//go:build !windows

package tray

func ensureSingleInstance() (func(), error) {
	return func() {}, nil
}

