package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"appcenter-agent/internal/config"
	"appcenter-agent/internal/downloader"
	"appcenter-agent/pkg/utils"
)

type StagedUpdate struct {
	Version      string `json:"version"`
	FilePath     string `json:"file_path"`
	Hash         string `json:"hash"`
	StagedAtUTC  string `json:"staged_at_utc"`
	SourceURL    string `json:"source_url"`
	AgentVersion string `json:"agent_version"`
}

func StageIfNeeded(
	ctx context.Context,
	cfg config.Config,
	hbConfig map[string]any,
	logger *log.Logger,
) error {
	latestVersion, _ := hbConfig["latest_agent_version"].(string)
	downloadURL, _ := hbConfig["agent_download_url"].(string)
	agentHash, _ := hbConfig["agent_hash"].(string)

	if latestVersion == "" || downloadURL == "" || agentHash == "" {
		return nil
	}
	if latestVersion == cfg.Agent.Version {
		return nil
	}

	if err := os.MkdirAll(cfg.Download.TempDir, 0o755); err != nil {
		return err
	}

	stagedPath := filepath.Join(cfg.Download.TempDir, fmt.Sprintf("agent-update-%s.exe", sanitizeVersion(latestVersion)))

	_, err := downloader.DownloadFileWithMeta(
		ctx,
		downloadURL,
		stagedPath,
		cfg.Download.BandwidthLimitKBs,
		cfg.Agent.UUID,
		cfg.Agent.SecretKey,
	)
	if err != nil {
		return fmt.Errorf("update download failed: %w", err)
	}

	valid, err := utils.VerifyFileHash(stagedPath, agentHash)
	if err != nil {
		return fmt.Errorf("update hash verify failed: %w", err)
	}
	if !valid {
		_ = os.Remove(stagedPath)
		return errors.New("update hash mismatch")
	}

	meta := StagedUpdate{
		Version:      latestVersion,
		FilePath:     stagedPath,
		Hash:         agentHash,
		StagedAtUTC:  time.Now().UTC().Format(time.RFC3339),
		SourceURL:    downloadURL,
		AgentVersion: cfg.Agent.Version,
	}
	metaPath := filepath.Join(cfg.Download.TempDir, "pending_update.json")
	b, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(metaPath, b, 0o600); err != nil {
		return fmt.Errorf("write pending update metadata: %w", err)
	}

	logger.Printf("self-update staged: current=%s target=%s file=%s", cfg.Agent.Version, latestVersion, stagedPath)
	return nil
}

func sanitizeVersion(v string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return r.Replace(v)
}
