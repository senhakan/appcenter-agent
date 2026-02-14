package config

import (
	"errors"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Agent     AgentConfig     `yaml:"agent"`
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`
	Download  DownloadConfig  `yaml:"download"`
	Install   InstallConfig   `yaml:"install"`
	WorkHours WorkHoursConfig `yaml:"work_hours"`
	Logging   LoggingConfig   `yaml:"logging"`
}

type ServerConfig struct {
	URL       string `yaml:"url"`
	VerifySSL bool   `yaml:"verify_ssl"`
}

type AgentConfig struct {
	Version   string `yaml:"version"`
	UUID      string `yaml:"uuid"`
	SecretKey string `yaml:"secret_key"`
}

type HeartbeatConfig struct {
	IntervalSec int `yaml:"interval_sec"`
}

type DownloadConfig struct {
	TempDir           string `yaml:"temp_dir"`
	BandwidthLimitKBs int    `yaml:"bandwidth_limit_kbps"`
}

type InstallConfig struct {
	TimeoutSec        int  `yaml:"timeout_sec"`
	EnableAutoCleanup bool `yaml:"enable_auto_cleanup"`
}

type WorkHoursConfig struct {
	StartUTC string `yaml:"start_utc"`
	EndUTC   string `yaml:"end_utc"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (c *Config) Validate() error {
	if c.Server.URL == "" {
		return errors.New("server.url is required")
	}
	if c.Heartbeat.IntervalSec <= 0 {
		return errors.New("heartbeat.interval_sec must be > 0")
	}
	if c.Download.BandwidthLimitKBs <= 0 {
		return errors.New("download.bandwidth_limit_kbps must be > 0")
	}
	if c.Install.TimeoutSec <= 0 {
		return errors.New("install.timeout_sec must be > 0")
	}
	if c.Agent.Version == "" {
		return errors.New("agent.version is required")
	}
	return nil
}
