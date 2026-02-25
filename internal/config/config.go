package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Agent         AgentConfig         `yaml:"agent"`
	Heartbeat     HeartbeatConfig     `yaml:"heartbeat"`
	SystemProfile SystemProfileConfig `yaml:"system_profile"`
	RemoteSupport RemoteSupportConfig `yaml:"remote_support"`
	Download      DownloadConfig      `yaml:"download"`
	Install       InstallConfig       `yaml:"install"`
	Update        UpdateConfig        `yaml:"update"`
	Logging       LoggingConfig       `yaml:"logging"`
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

type SystemProfileConfig struct {
	// ReportIntervalMin controls how often system profile (static-ish machine info)
	// is sent to the server. It is intentionally not sent on every heartbeat.
	ReportIntervalMin int `yaml:"report_interval_min"`
}

type DownloadConfig struct {
	TempDir           string `yaml:"temp_dir"`
	BandwidthLimitKBs int    `yaml:"bandwidth_limit_kbps"`
}

type InstallConfig struct {
	TimeoutSec        int  `yaml:"timeout_sec"`
	EnableAutoCleanup bool `yaml:"enable_auto_cleanup"`
}

type UpdateConfig struct {
	// AutoApply enables applying staged updates (pending_update.json) on the next idle loop.
	AutoApply bool `yaml:"auto_apply"`
	// ServiceName is only used by Windows update helper to restart the service.
	ServiceName string `yaml:"service_name"`
	// HelperPath is the path to the update helper executable on Windows.
	HelperPath string `yaml:"helper_path"`
}

type RemoteSupportConfig struct {
	Enabled            bool `yaml:"enabled"`
	ApprovalTimeoutSec int  `yaml:"approval_timeout_sec"`
}

type LoggingConfig struct {
	Level      string `yaml:"level"`
	File       string `yaml:"file"`
	MaxSizeMB  int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
}

func Default() *Config {
	// Keep this aligned with configs/config.yaml.template (but generated programmatically so
	// the service can recover even if the file is missing on disk).
	return &Config{
		Server: ServerConfig{
			URL:       "http://10.6.100.170:8000",
			VerifySSL: false,
		},
		Agent: AgentConfig{
			Version:   "0.0.0",
			UUID:      "",
			SecretKey: "",
		},
		Heartbeat: HeartbeatConfig{IntervalSec: 60},
		SystemProfile: SystemProfileConfig{
			ReportIntervalMin: 720,
		},
		RemoteSupport: RemoteSupportConfig{
			Enabled:            false,
			ApprovalTimeoutSec: 30,
		},
		Download: DownloadConfig{
			TempDir:           `C:\ProgramData\AppCenter\downloads`,
			BandwidthLimitKBs: 1024,
		},
		Install: InstallConfig{
			TimeoutSec:        1800,
			EnableAutoCleanup: true,
		},
		Update: UpdateConfig{
			AutoApply:   true,
			ServiceName: "AppCenterAgent",
			HelperPath:  `C:\Program Files\AppCenter\appcenter-update-helper.exe`,
		},
		Logging: LoggingConfig{
			Level:      "info",
			File:       `C:\ProgramData\AppCenter\logs\agent.log`,
			MaxSizeMB:  10,
			MaxBackups: 5,
		},
	}
}

// EnsureExists creates a default config file when it does not exist.
// It never overwrites an existing config.
func EnsureExists(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(Default())
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
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
	cfg.ApplyDefaults()
	cfg.ApplyRuntimeOverrides()
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
	if c.Logging.MaxSizeMB <= 0 {
		return errors.New("logging.max_size_mb must be > 0")
	}
	if c.Logging.MaxBackups <= 0 {
		return errors.New("logging.max_backups must be > 0")
	}
	if c.Agent.Version == "" {
		return errors.New("agent.version is required")
	}
	if c.SystemProfile.ReportIntervalMin < 0 {
		return errors.New("system_profile.report_interval_min must be >= 0")
	}
	if c.RemoteSupport.ApprovalTimeoutSec <= 0 {
		return errors.New("remote_support.approval_timeout_sec must be > 0")
	}
	return nil
}

func (c *Config) ApplyDefaults() {
	if c.Update.ServiceName == "" {
		c.Update.ServiceName = "AppCenterAgent"
	}
	if c.SystemProfile.ReportIntervalMin == 0 {
		c.SystemProfile.ReportIntervalMin = 720
	}
	if c.RemoteSupport.ApprovalTimeoutSec == 0 {
		c.RemoteSupport.ApprovalTimeoutSec = 30
	}
}
