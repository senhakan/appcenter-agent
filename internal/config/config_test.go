package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, dir string, serverURL string, secret string) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	content := "server:\n" +
		"  url: \"" + serverURL + "\"\n" +
		"  verify_ssl: false\n" +
		"agent:\n" +
		"  version: \"0.1.0\"\n" +
		"  uuid: \"\"\n" +
		"  secret_key: \"" + secret + "\"\n" +
		"heartbeat:\n" +
		"  interval_sec: 60\n" +
		"download:\n" +
		"  temp_dir: \"/tmp\"\n" +
		"  bandwidth_limit_kbps: 100\n" +
		"install:\n" +
		"  timeout_sec: 300\n" +
		"  enable_auto_cleanup: true\n" +
		"update:\n" +
		"  auto_apply: false\n" +
		"  service_name: \"AppCenterAgent\"\n" +
		"  helper_path: \"\"\n" +
		"work_hours:\n" +
		"  start_utc: \"09:00\"\n" +
		"  end_utc: \"18:00\"\n" +
		"logging:\n" +
		"  level: \"info\"\n" +
		"  file: \"/tmp/agent.log\"\n" +
		"  max_size_mb: 10\n" +
		"  max_backups: 3\n"
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoadAppliesEnvOverrides(t *testing.T) {
	t.Setenv("APPCENTER_SERVER_URL", "https://override.example")
	t.Setenv("APPCENTER_SECRET_KEY", "env-secret")

	p := writeTestConfig(t, t.TempDir(), "http://127.0.0.1:8000", "file-secret")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if cfg.Server.URL != "https://override.example" {
		t.Fatalf("expected server url override, got %q", cfg.Server.URL)
	}
	if cfg.Agent.SecretKey != "env-secret" {
		t.Fatalf("expected secret override, got %q", cfg.Agent.SecretKey)
	}
}

