package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

var semverLikeRe = regexp.MustCompile(`\d+(?:\.\d+){0,2}`)

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
	// Avoid downgrades or re-staging the same/older version if server config is behind.
	if !isNewerVersion(latestVersion, cfg.Agent.Version) {
		return nil
	}

	if err := os.MkdirAll(cfg.Download.TempDir, 0o755); err != nil {
		return err
	}

	// Server may provide relative URLs; resolve against configured server.url.
	resolvedURL := downloadURL
	if strings.HasPrefix(resolvedURL, "/") {
		resolvedURL = strings.TrimRight(cfg.Server.URL, "/") + resolvedURL
	}

	stagedPath := filepath.Join(cfg.Download.TempDir, fmt.Sprintf("agent-update-%s.exe", sanitizeVersion(latestVersion)))

	_, err := downloader.DownloadFileWithMeta(
		ctx,
		resolvedURL,
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
		SourceURL:    resolvedURL,
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

// isNewerVersion compares two version strings and returns true if target > current.
// We intentionally support sloppy inputs (e.g. "1.2.3 / build:7") by extracting the
// first semver-like token and comparing numerically.
func isNewerVersion(target string, current string) bool {
	t := extractSemver3(target)
	c := extractSemver3(current)
	if t == nil || c == nil {
		// If parsing fails, keep the old behavior (don't surprise by forcing updates).
		return target != "" && target != current
	}
	if t[0] != c[0] {
		return t[0] > c[0]
	}
	if t[1] != c[1] {
		return t[1] > c[1]
	}
	return t[2] > c[2]
}

func extractSemver3(v string) []int {
	m := semverLikeRe.FindString(v)
	if m == "" {
		return nil
	}
	parts := strings.Split(m, ".")
	out := []int{0, 0, 0}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return nil
		}
		out[i] = n
	}
	return out
}
