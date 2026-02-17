package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"appcenter-agent/internal/config"
	"appcenter-agent/internal/system"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg config.ServerConfig) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type RegisterRequest struct {
	UUID         string `json:"uuid"`
	Hostname     string `json:"hostname"`
	OSVersion    string `json:"os_version"`
	AgentVersion string `json:"agent_version"`
	CPUModel     string `json:"cpu_model"`
	RAMGB        int    `json:"ram_gb"`
	DiskFreeGB   int    `json:"disk_free_gb"`
}

type RegisterResponse struct {
	Status    string         `json:"status"`
	Message   string         `json:"message"`
	SecretKey string         `json:"secret_key"`
	Config    map[string]any `json:"config"`
}

type InstalledApp struct {
	AppID   int    `json:"app_id"`
	Version string `json:"version"`
}

type HeartbeatRequest struct {
	Hostname      string         `json:"hostname"`
	IPAddress     string         `json:"ip_address"`
	OSUser        string         `json:"os_user"`
	AgentVersion  string         `json:"agent_version"`
	DiskFreeGB    int            `json:"disk_free_gb"`
	CPUUsage      float64        `json:"cpu_usage"`
	RAMUsage      float64        `json:"ram_usage"`
	CurrentStatus string         `json:"current_status"`
	AppsChanged   bool           `json:"apps_changed"`
	InstalledApps []InstalledApp `json:"installed_apps"`
	InventoryHash string         `json:"inventory_hash,omitempty"`
}

type Command struct {
	TaskID        int    `json:"task_id"`
	Action        string `json:"action"`
	AppID         int    `json:"app_id"`
	AppName       string `json:"app_name"`
	AppVersion    string `json:"app_version"`
	DownloadURL   string `json:"download_url"`
	FileHash      string `json:"file_hash"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	InstallArgs   string `json:"install_args"`
	ForceUpdate   bool   `json:"force_update"`
	Priority      int    `json:"priority"`
}

type HeartbeatResponse struct {
	Status     string         `json:"status"`
	ServerTime string         `json:"server_time"`
	Config     map[string]any `json:"config"`
	Commands   []Command      `json:"commands"`
}

type TaskStatusRequest struct {
	Status              string `json:"status"`
	Progress            int    `json:"progress"`
	Message             string `json:"message"`
	// Use pointer so `0` is not dropped by `omitempty` when we want to persist success exit codes.
	ExitCode            *int   `json:"exit_code,omitempty"`
	InstalledVersion    string `json:"installed_version,omitempty"`
	DownloadDurationSec int    `json:"download_duration_sec,omitempty"`
	InstallDurationSec  int    `json:"install_duration_sec,omitempty"`
	Error               string `json:"error,omitempty"`
}

type TaskStatusResponse struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type StoreApp struct {
	ID               int    `json:"id"`
	DisplayName      string `json:"display_name"`
	Version          string `json:"version"`
	Description      string `json:"description"`
	IconURL          string `json:"icon_url"`
	FileSizeMB       int    `json:"file_size_mb"`
	Category         string `json:"category"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installed_version"`
	CanUninstall     bool   `json:"can_uninstall"`
}

type StoreResponse struct {
	Apps []StoreApp `json:"apps"`
}

func (c *Client) Register(ctx context.Context, uuid string, version string, info system.HostInfo) (*RegisterResponse, error) {
	payload := RegisterRequest{
		UUID:         uuid,
		Hostname:     info.Hostname,
		OSVersion:    info.OSVersion,
		AgentVersion: version,
		CPUModel:     info.CPUModel,
		RAMGB:        info.RAMGB,
		DiskFreeGB:   info.DiskFreeGB,
	}

	var out RegisterResponse
	if err := c.postJSON(ctx, "/api/v1/agent/register", payload, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Heartbeat(ctx context.Context, agentUUID, secret string, reqBody HeartbeatRequest) (*HeartbeatResponse, error) {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}

	var out HeartbeatResponse
	if err := c.postJSON(ctx, "/api/v1/agent/heartbeat", reqBody, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ReportTaskStatus(
	ctx context.Context,
	agentUUID,
	secret string,
	taskID int,
	reqBody TaskStatusRequest,
) (*TaskStatusResponse, error) {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}

	var out TaskStatusResponse
	path := fmt.Sprintf("/api/v1/agent/task/%d/status", taskID)
	if err := c.postJSON(ctx, path, reqBody, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetStore(ctx context.Context, agentUUID, secret string) (*StoreResponse, error) {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}

	var out StoreResponse
	if err := c.getJSON(ctx, "/api/v1/agent/store", headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SubmitInventory(ctx context.Context, agentUUID, secret string, payload any) (map[string]any, error) {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}

	var out map[string]any
	if err := c.postJSON(ctx, "/api/v1/agent/inventory", payload, headers, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, headers map[string]string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) getJSON(ctx context.Context, path string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
