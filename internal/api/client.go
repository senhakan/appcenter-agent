package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"appcenter-agent/internal/config"
	"appcenter-agent/internal/system"
)

type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Status     string
	Detail     string
	Body       string
}

func (e *HTTPError) Error() string {
	msg := e.Status
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s %s: %s (%s)", e.Method, e.URL, e.Detail, msg)
	}
	if e.Body != "" {
		return fmt.Sprintf("%s %s: %s (%s)", e.Method, e.URL, e.Body, msg)
	}
	return fmt.Sprintf("%s %s failed: %s", e.Method, e.URL, msg)
}

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

type LoggedInSession struct {
	Username    string `json:"username"`
	SessionType string `json:"session_type"`
	LogonID     string `json:"logon_id,omitempty"`
}

type SystemDisk struct {
	Index   int    `json:"index"`
	SizeGB  int    `json:"size_gb,omitempty"`
	Model   string `json:"model,omitempty"`
	BusType string `json:"bus_type,omitempty"`
}

type VirtualizationInfo struct {
	IsVirtual bool   `json:"is_virtual"`
	Vendor    string `json:"vendor,omitempty"`
	Model     string `json:"model,omitempty"`
}

type SystemProfile struct {
	OSFullName   string `json:"os_full_name,omitempty"`
	OSVersion    string `json:"os_version,omitempty"`
	BuildNumber  string `json:"build_number,omitempty"`
	Architecture string `json:"architecture,omitempty"`

	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`

	CPUModel         string `json:"cpu_model,omitempty"`
	CPUCoresPhysical int    `json:"cpu_cores_physical,omitempty"`
	CPUCoresLogical  int    `json:"cpu_cores_logical,omitempty"`
	TotalMemoryGB    int    `json:"total_memory_gb,omitempty"`

	DiskCount int          `json:"disk_count,omitempty"`
	Disks     []SystemDisk `json:"disks,omitempty"`

	Virtualization *VirtualizationInfo `json:"virtualization,omitempty"`
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
	// Always send this field so the server can clear stale session data.
	LoggedInSessions []LoggedInSession    `json:"logged_in_sessions"`
	SystemProfile    *SystemProfile       `json:"system_profile,omitempty"`
	RemoteSupport    *RemoteSupportStatus `json:"remote_support,omitempty"`
}

type RemoteSupportStatus struct {
	State         string `json:"state"`
	SessionID     int    `json:"session_id"`
	HelperRunning bool   `json:"helper_running"`
	HelperPID     int    `json:"helper_pid"`
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
	Status               string                `json:"status"`
	ServerTime           string                `json:"server_time"`
	Config               map[string]any        `json:"config"`
	Commands             []Command             `json:"commands"`
	RemoteSupportRequest *RemoteSupportRequest `json:"remote_support_request,omitempty"`
	RemoteSupportEnd     *RemoteSupportEnd     `json:"remote_support_end,omitempty"`
}

type RemoteSupportRequest struct {
	SessionID   int    `json:"session_id"`
	AdminName   string `json:"admin_name"`
	Reason      string `json:"reason"`
	RequestedAt string `json:"requested_at"`
	TimeoutAt   string `json:"timeout_at"`
}

type RemoteSupportEnd struct {
	SessionID int `json:"session_id"`
}

type ApproveRemoteSessionResponse struct {
	Status       string `json:"status"`
	Message      string `json:"message,omitempty"`
	VNCPassword  string `json:"vnc_password,omitempty"`
	GuacdHost    string `json:"guacd_host,omitempty"`
	GuacdRevPort int    `json:"guacd_reverse_port,omitempty"`
}

type TaskStatusRequest struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
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

type MessageResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type StoreApp struct {
	ID                 int    `json:"id"`
	DisplayName        string `json:"display_name"`
	Version            string `json:"version"`
	Description        string `json:"description"`
	IconURL            string `json:"icon_url"`
	FileSizeMB         int    `json:"file_size_mb"`
	Category           string `json:"category"`
	Installed          bool   `json:"installed"`
	InstallState       string `json:"install_state"`
	ErrorMessage       string `json:"error_message"`
	ConflictDetected   bool   `json:"conflict_detected"`
	ConflictConfidence string `json:"conflict_confidence"`
	ConflictMessage    string `json:"conflict_message"`
	InstalledVersion   string `json:"installed_version"`
	CanUninstall       bool   `json:"can_uninstall"`
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

func (c *Client) RequestStoreInstall(ctx context.Context, agentUUID, secret string, appID int) (*MessageResponse, error) {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}

	var out MessageResponse
	path := fmt.Sprintf("/api/v1/agent/store/%d/install", appID)
	if err := c.postJSON(ctx, path, map[string]any{}, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ApproveRemoteSession(
	ctx context.Context,
	agentUUID, secret string,
	sessionID int,
	approved bool,
) (*ApproveRemoteSessionResponse, error) {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}

	var out ApproveRemoteSessionResponse
	path := fmt.Sprintf("/api/v1/agent/remote-support/%d/approve", sessionID)
	if err := c.postJSON(ctx, path, map[string]bool{"approved": approved}, headers, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ReportRemoteReady(
	ctx context.Context,
	agentUUID, secret string,
	sessionID int,
) error {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}
	path := fmt.Sprintf("/api/v1/agent/remote-support/%d/ready", sessionID)
	return c.postJSON(ctx, path, map[string]any{"vnc_ready": true}, headers, &MessageResponse{})
}

func (c *Client) ReportRemoteEnded(
	ctx context.Context,
	agentUUID, secret string,
	sessionID int,
	endedBy string,
) error {
	headers := map[string]string{
		"X-Agent-UUID":   agentUUID,
		"X-Agent-Secret": secret,
	}
	path := fmt.Sprintf("/api/v1/agent/remote-support/%d/ended", sessionID)
	return c.postJSON(ctx, path, map[string]string{"ended_by": endedBy}, headers, &MessageResponse{})
}

func (c *Client) postJSON(ctx context.Context, path string, payload any, headers map[string]string, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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
		return httpErrorFromResponse(http.MethodPost, url, resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) getJSON(ctx context.Context, path string, headers map[string]string, out any) error {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
		return httpErrorFromResponse(http.MethodGet, url, resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func httpErrorFromResponse(method, url string, resp *http.Response) error {
	// Bound memory usage; we only need a small snippet for diagnostics.
	const maxBody = 64 * 1024
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	body := strings.TrimSpace(string(b))

	// Server standard: {"status":"error","detail":"..."} (or older {"message": "..."}).
	var apiErr struct {
		Status  string `json:"status"`
		Detail  string `json:"detail"`
		Message string `json:"message"`
	}
	detail := ""
	if len(b) > 0 && json.Unmarshal(b, &apiErr) == nil {
		if apiErr.Detail != "" {
			detail = apiErr.Detail
		} else if apiErr.Message != "" {
			detail = apiErr.Message
		}
	}

	return &HTTPError{
		Method:     method,
		URL:        url,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Detail:     detail,
		Body:       body,
	}
}
