//go:build windows

package system

import (
	"github.com/google/uuid"
	"golang.org/x/sys/windows/registry"
)

const appCenterRegistryPath = `SOFTWARE\\AppCenter`

func GetOrCreateUUID() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, appCenterRegistryPath, registry.QUERY_VALUE)
	if err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("UUID"); err == nil && v != "" {
			return v, nil
		}
	}

	newUUID := uuid.NewString()

	k, _, err = registry.CreateKey(registry.LOCAL_MACHINE, appCenterRegistryPath, registry.SET_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	if err := k.SetStringValue("UUID", newUUID); err != nil {
		return "", err
	}
	return newUUID, nil
}
