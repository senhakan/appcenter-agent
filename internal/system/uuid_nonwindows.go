//go:build !windows

package system

import "github.com/google/uuid"

func GetOrCreateUUID() (string, error) {
	return uuid.NewString(), nil
}
