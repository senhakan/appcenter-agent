//go:build !windows

package ipc

import "errors"

func StartPipeServer(_ Handler) (Server, error) {
	return nil, errors.New("named pipes are only supported on windows")
}

func SendRequest(_ Request) (*Response, error) {
	return nil, errors.New("named pipes are only supported on windows")
}
