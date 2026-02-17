package config

import "os"

// ApplyRuntimeOverrides applies environment/OS-specific overrides after YAML load
// and before validation.
func (c *Config) ApplyRuntimeOverrides() {
	applyEnvOverrides(c)
	applyOSOverrides(c)
}

func applyEnvOverrides(c *Config) {
	if v := os.Getenv("APPCENTER_SERVER_URL"); v != "" {
		c.Server.URL = v
	}
	if v := os.Getenv("APPCENTER_SECRET_KEY"); v != "" {
		c.Agent.SecretKey = v
	}
}

