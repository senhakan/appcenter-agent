// Package inventory handles software inventory scanning, hashing and sync with the server.
package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// SoftwareItem represents a single installed software entry.
type SoftwareItem struct {
	Name            string `json:"name"`
	Version         string `json:"version,omitempty"`
	Publisher       string `json:"publisher,omitempty"`
	InstallDate     string `json:"install_date,omitempty"`
	EstimatedSizeKB int    `json:"estimated_size_kb,omitempty"`
	Architecture    string `json:"architecture,omitempty"`
}

// SubmitRequest is the payload sent to POST /api/v1/agent/inventory.
type SubmitRequest struct {
	InventoryHash string         `json:"inventory_hash"`
	SoftwareCount int            `json:"software_count"`
	Items         []SoftwareItem `json:"items"`
}

// SubmitResponse is the server response for inventory submission.
type SubmitResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message"`
	Changes map[string]int `json:"changes"`
}

// Manager handles periodic inventory scanning and server synchronization.
type Manager struct {
	mu              sync.Mutex
	logger          *log.Logger
	lastHash        string
	lastItems       []SoftwareItem
	scanIntervalMin int
	lastScanTime    time.Time
}

// NewManager creates a new inventory manager.
func NewManager(logger *log.Logger) *Manager {
	return &Manager{
		logger:          logger,
		scanIntervalMin: 10,
	}
}

// SetScanInterval updates the scan interval from server config.
func (m *Manager) SetScanInterval(minutes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if minutes > 0 {
		m.scanIntervalMin = minutes
	}
}

// GetCurrentHash returns the current inventory hash (thread-safe).
func (m *Manager) GetCurrentHash() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastHash
}

// ScanIfNeeded performs a scan if enough time has elapsed since the last one.
// Returns true if a new scan was performed.
func (m *Manager) ScanIfNeeded() bool {
	m.mu.Lock()
	interval := time.Duration(m.scanIntervalMin) * time.Minute
	needsScan := time.Since(m.lastScanTime) >= interval
	m.mu.Unlock()

	if !needsScan {
		return false
	}

	items := ScanInstalledSoftware()
	hash := computeHash(items)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastItems = items
	m.lastHash = hash
	m.lastScanTime = time.Now()
	m.logger.Printf("inventory scan complete: %d items, hash=%s", len(items), hash[:12])
	return true
}

// ForceScan performs an immediate scan regardless of interval.
func (m *Manager) ForceScan() {
	items := ScanInstalledSoftware()
	hash := computeHash(items)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastItems = items
	m.lastHash = hash
	m.lastScanTime = time.Now()
	m.logger.Printf("inventory force scan: %d items, hash=%s", len(items), hash[:12])
}

// GetSubmitPayload returns the current inventory as a SubmitRequest.
func (m *Manager) GetSubmitPayload() SubmitRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]SoftwareItem, len(m.lastItems))
	copy(items, m.lastItems)
	return SubmitRequest{
		InventoryHash: m.lastHash,
		SoftwareCount: len(items),
		Items:         items,
	}
}

// SubmitFunc is the function signature for submitting inventory to the server.
type SubmitFunc func(ctx context.Context, payload SubmitRequest) (*SubmitResponse, error)

// SyncIfRequested submits inventory to the server if sync is required.
func (m *Manager) SyncIfRequested(ctx context.Context, syncRequired bool, submitFn SubmitFunc) {
	if !syncRequired {
		return
	}

	payload := m.GetSubmitPayload()
	if len(payload.Items) == 0 {
		m.logger.Println("inventory sync requested but no items to send")
		return
	}

	resp, err := submitFn(ctx, payload)
	if err != nil {
		m.logger.Printf("inventory submit failed: %v", err)
		return
	}
	m.logger.Printf("inventory submitted: %s (installed=%d removed=%d updated=%d)",
		resp.Message,
		resp.Changes["installed"],
		resp.Changes["removed"],
		resp.Changes["updated"],
	)
}

// computeHash sorts items by name, serializes to JSON, and returns SHA256 hex.
func computeHash(items []SoftwareItem) string {
	if len(items) == 0 {
		return "empty"
	}

	// Sort by name for deterministic hash
	sorted := make([]SoftwareItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	data, _ := json.Marshal(sorted)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
