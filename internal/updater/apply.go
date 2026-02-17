package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"appcenter-agent/internal/config"
	"appcenter-agent/pkg/utils"
)

var ErrUpdateRestart = errors.New("update applied; service should restart")

type processRunner interface {
	Start(cmd *exec.Cmd) error
}

type defaultRunner struct{}

func (defaultRunner) Start(cmd *exec.Cmd) error { return cmd.Start() }

func pendingMetaPath(cfg config.Config) string {
	return filepath.Join(cfg.Download.TempDir, "pending_update.json")
}

func loadStagedUpdate(path string) (StagedUpdate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return StagedUpdate{}, err
	}
	var m StagedUpdate
	if err := json.Unmarshal(b, &m); err != nil {
		return StagedUpdate{}, err
	}
	if m.Version == "" || m.FilePath == "" || m.Hash == "" {
		return StagedUpdate{}, errors.New("invalid pending update metadata")
	}
	return m, nil
}

func ApplyIfPending(
	ctx context.Context,
	cfg config.Config,
	cfgPath string,
	serviceExePath string,
	logger *log.Logger,
) error {
	return applyIfPending(ctx, cfg, cfgPath, serviceExePath, logger, defaultRunner{})
}

func applyIfPending(
	ctx context.Context,
	cfg config.Config,
	cfgPath string,
	serviceExePath string,
	logger *log.Logger,
	runner processRunner,
) error {
	if !cfg.Update.AutoApply {
		return nil
	}

	metaPath := pendingMetaPath(cfg)
	if _, err := os.Stat(metaPath); err != nil {
		return nil
	}

	meta, err := loadStagedUpdate(metaPath)
	if err != nil {
		return fmt.Errorf("read pending update: %w", err)
	}
	// Never apply downgrades or no-ops. Pending metadata may be stale if server
	// configuration was rolled back or if the agent was upgraded via MSI.
	if !isNewerVersion(meta.Version, cfg.Agent.Version) {
		// Already at target version; metadata is stale.
		return nil
	}

	// Re-verify before applying.
	valid, err := utils.VerifyFileHash(meta.FilePath, meta.Hash)
	if err != nil {
		return fmt.Errorf("verify staged update hash: %w", err)
	}
	if !valid {
		return errors.New("staged update hash mismatch")
	}

	helper := cfg.Update.HelperPath
	if helper == "" {
		return errors.New("update.helper_path is required when update.auto_apply=true")
	}
	if serviceExePath == "" {
		return errors.New("service exe path is empty")
	}

	// Do NOT bind helper lifecycle to ctx; we intentionally exit the service after spawning it.
	cmd := exec.Command(helper,
		"--service-name", cfg.Update.ServiceName,
		"--target-exe", serviceExePath,
		"--new-exe", meta.FilePath,
		"--meta", metaPath,
		"--config", cfgPath,
		"--target-version", meta.Version,
	)

	// Run detached; helper will stop/start the service.
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := runner.Start(cmd); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}

	logger.Printf("self-update apply triggered: helper=%s target=%s new=%s version=%s", helper, filepath.Base(serviceExePath), filepath.Base(meta.FilePath), meta.Version)
	return ErrUpdateRestart
}
