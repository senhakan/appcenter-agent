package runtimeupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"appcenter-agent/pkg/utils"
)

const (
	defaultIntervalMin = 60
	defaultJitterSec   = 300
)

type Config struct {
	BaseURL     string
	IntervalMin int
	JitterSec   int
}

type Manifest struct {
	Version string                  `json:"version"`
	Files   map[string]ManifestFile `json:"files"`
}

type ManifestFile struct {
	SHA256 string `json:"sha256"`
	URL    string `json:"url,omitempty"`
}

type Manager struct {
	mu sync.RWMutex

	cfg      Config
	exeDir   string
	logger   *log.Logger
	client   *http.Client
	onTrayUp func()
	wakeCh   chan struct{}
}

func NewManager(exeDir string, logger *log.Logger, onTrayUpdated func()) *Manager {
	return &Manager{
		cfg: Config{
			BaseURL:     "",
			IntervalMin: defaultIntervalMin,
			JitterSec:   defaultJitterSec,
		},
		exeDir:   exeDir,
		logger:   logger,
		client:   &http.Client{Timeout: 30 * time.Second},
		onTrayUp: onTrayUpdated,
		wakeCh:   make(chan struct{}, 1),
	}
}

func (m *Manager) UpdateConfig(cfg Config) {
	cfg = normalizeConfig(cfg)
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

func (m *Manager) Start(ctx context.Context) {
	m.logger.Printf("runtime update: loop started")
	for {
		select {
		case <-ctx.Done():
			m.logger.Printf("runtime update: loop stopped")
			return
		default:
		}

		cfg := m.snapshotConfig()
		if cfg.BaseURL == "" {
			select {
			case <-ctx.Done():
				return
			case <-m.wakeCh:
				continue
			case <-time.After(1 * time.Minute):
				continue
			}
		}

		if err := m.syncOnce(ctx, cfg); err != nil {
			m.logger.Printf("runtime update: sync failed: %v", err)
		}

		waitDur := computeWait(cfg)
		select {
		case <-ctx.Done():
			return
		case <-m.wakeCh:
			continue
		case <-time.After(waitDur):
		}
	}
}

func normalizeConfig(cfg Config) Config {
	cfg.BaseURL = strings.TrimSpace(cfg.BaseURL)
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.IntervalMin <= 0 {
		cfg.IntervalMin = defaultIntervalMin
	}
	if cfg.JitterSec < 0 {
		cfg.JitterSec = 0
	}
	return cfg
}

func computeWait(cfg Config) time.Duration {
	base := time.Duration(cfg.IntervalMin) * time.Minute
	if cfg.JitterSec <= 0 {
		return base
	}
	seed := time.Now().UnixNano()
	r := rand.New(rand.NewSource(seed))
	j := time.Duration(r.Intn(cfg.JitterSec+1)) * time.Second
	return base + j
}

func (m *Manager) snapshotConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) syncOnce(ctx context.Context, cfg Config) error {
	manifestURL := cfg.BaseURL + "/manifest.json"
	manifest, err := m.fetchManifest(ctx, manifestURL)
	if err != nil {
		return err
	}

	if err := m.ensureFile(ctx, cfg, manifest, "appcenter-tray.exe"); err != nil {
		return err
	}
	if err := m.ensureFile(ctx, cfg, manifest, "rshelper.exe"); err != nil {
		return err
	}
	return nil
}

func (m *Manager) fetchManifest(ctx context.Context, manifestURL string) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest download failed: status=%d", resp.StatusCode)
	}
	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("manifest decode failed: %w", err)
	}
	if manifest.Files == nil {
		manifest.Files = map[string]ManifestFile{}
	}
	return &manifest, nil
}

func (m *Manager) ensureFile(ctx context.Context, cfg Config, manifest *Manifest, name string) error {
	entry, ok := manifest.Files[name]
	if !ok {
		m.logger.Printf("runtime update: %s missing in manifest, skip", name)
		return nil
	}
	expected := strings.TrimSpace(entry.SHA256)
	if expected == "" {
		m.logger.Printf("runtime update: %s hash empty in manifest, skip", name)
		return nil
	}

	dstPath := filepath.Join(m.exeDir, name)
	matches, err := utils.VerifyFileHash(dstPath, expected)
	if err == nil && matches {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		m.logger.Printf("runtime update: local hash check failed for %s: %v", name, err)
	}

	fileURL := strings.TrimSpace(entry.URL)
	if fileURL == "" {
		fileURL = cfg.BaseURL + "/" + name
	}
	tempPath := dstPath + ".download"
	if err := downloadToFile(ctx, m.client, fileURL, tempPath); err != nil {
		return fmt.Errorf("%s download failed: %w", name, err)
	}
	defer os.Remove(tempPath)

	okHash, err := utils.VerifyFileHash(tempPath, expected)
	if err != nil {
		return fmt.Errorf("%s downloaded hash check failed: %w", name, err)
	}
	if !okHash {
		return fmt.Errorf("%s hash mismatch after download", name)
	}

	if strings.EqualFold(name, "appcenter-tray.exe") {
		stopTrayProcess()
	}
	if strings.EqualFold(name, "rshelper.exe") && isProcessRunningByImage("rshelper.exe") {
		m.logger.Printf("runtime update: rshelper is running, defer replacement")
		return nil
	}

	if err := replaceFile(dstPath, tempPath); err != nil {
		return fmt.Errorf("%s replace failed: %w", name, err)
	}

	m.logger.Printf("runtime update: %s updated", name)
	if strings.EqualFold(name, "appcenter-tray.exe") && m.onTrayUp != nil {
		m.onTrayUp()
	}
	return nil
}

func downloadToFile(ctx context.Context, client *http.Client, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status=%d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}

func replaceFile(dstPath, tempPath string) error {
	backupPath := dstPath + ".bak"
	_ = os.Remove(backupPath)
	if _, err := os.Stat(dstPath); err == nil {
		if err := os.Rename(dstPath, backupPath); err != nil {
			_ = os.Remove(dstPath)
		}
	}
	if err := os.Rename(tempPath, dstPath); err != nil {
		if _, stErr := os.Stat(backupPath); stErr == nil {
			_ = os.Rename(backupPath, dstPath)
		}
		return err
	}
	return nil
}
