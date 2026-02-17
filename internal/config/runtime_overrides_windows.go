//go:build windows

package config

import "golang.org/x/sys/windows/registry"

const installerBootstrapRegistryPath = `SOFTWARE\\AppCenter\\Agent\\Bootstrap`

func applyOSOverrides(c *Config) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, installerBootstrapRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer k.Close()

	if v, _, err := k.GetStringValue("ServerURL"); err == nil && v != "" {
		c.Server.URL = v
	}
	if v, _, err := k.GetStringValue("SecretKey"); err == nil && v != "" {
		c.Agent.SecretKey = v
	}
}
