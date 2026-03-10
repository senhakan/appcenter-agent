package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"appcenter-agent/internal/announcement"
	"appcenter-agent/internal/api"
	"appcenter-agent/internal/config"
	"appcenter-agent/internal/downloader"
	"appcenter-agent/internal/heartbeat"
	"appcenter-agent/internal/installer"
	"appcenter-agent/internal/inventory"
	"appcenter-agent/internal/ipc"
	"appcenter-agent/internal/queue"
	"appcenter-agent/internal/remotesupport"
	"appcenter-agent/internal/runtimeupdate"
	"appcenter-agent/internal/system"
	"appcenter-agent/internal/updater"
	"appcenter-agent/internal/wsconn"
	"appcenter-agent/pkg/utils"
)

const serviceName = "AppCenterAgent"

func runAgent(ctx context.Context, cfgPath string) error {
	// MSI upgrades/uninstalls and manual tampering can leave config.yaml missing.
	// The service should be able to recover by re-creating a sane default config.
	if err := config.EnsureExists(cfgPath); err != nil {
		return fmt.Errorf("failed to ensure config exists: %w", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger, logCloser, err := utils.NewLogger(
		logPathOrFallback(cfg.Logging.File),
		cfg.Logging.MaxSizeMB,
		cfg.Logging.MaxBackups,
	)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer logCloser.Close()

	client := api.NewClient(cfg.Server)
	if err := bootstrapAgent(client, cfg, logger); err != nil {
		// Do not fail service start just because server is unreachable or registration fails.
		// If we exit here, Windows SCM shows "Error 1: Incorrect function" which is misleading.
		// Keep the agent running and let heartbeat retry; tray can show orange (server offline).
		logger.Printf("bootstrap warning (service will continue): %v", err)
	}

	serviceExe, _ := os.Executable()
	ensureRemoteSupportFirewallRules(filepath.Dir(serviceExe), logger)
	traySup := newTraySupervisor(serviceExe, logger)
	var storeTrayEnabled atomic.Bool
	var remoteSupportEnabled atomic.Bool

	runtimeMgr := runtimeupdate.NewManager(filepath.Dir(serviceExe), logger, func() {
		if storeTrayEnabled.Load() {
			traySup.SetEnabled(true)
		}
	})
	go runtimeMgr.Start(ctx)

	taskQueue := queue.NewTaskQueue(3)
	pollResults := make(chan heartbeat.PollResult, 8)
	serviceStarted := time.Now().UTC()

	// Initialize inventory manager and perform initial scan.
	invManager := inventory.NewManager(logger)
	invManager.ForceScan()

	// Remote support session manager stays available regardless of local config flag.
	// The server controls whether requests are sent; if no request arrives, manager is idle.
	sessionMgr := remotesupport.NewSessionManager(
		client,
		cfg.Agent.UUID,
		cfg.Agent.SecretKey,
		cfg.RemoteSupport.ApprovalTimeoutSec,
		logger,
	)
	logger.Printf("remote support: manager ready")
	remoteSupportEnabled.Store(cfg.RemoteSupport.Enabled)
	logger.Printf("remote support: enabled=%t (initial)", remoteSupportEnabled.Load())
	var remoteProvider heartbeat.RemoteSupportProvider
	if sessionMgr != nil {
		remoteProvider = sessionMgr
	}

	pipeServer, pipeErr := ipc.StartPipeServer(buildIPCHandler(client, cfg, taskQueue, logger, serviceStarted, sessionMgr, &remoteSupportEnabled))
	if pipeErr != nil {
		logger.Printf("named pipe server not started: %v", pipeErr)
	} else {
		defer pipeServer.Close()
		logger.Printf("named pipe server started: %s", ipc.PipeName)
	}

	sender := heartbeat.NewSender(client, cfg, logger, pollResults, taskQueue, invManager, remoteProvider)
	var wsActive atomic.Bool
	sender.SetWSActive(false)
	go sender.Start(ctx)
	wsInventoryKickCh := make(chan struct{}, 1)
	wsInventoryTicker := time.NewTicker(1 * time.Minute)
	defer wsInventoryTicker.Stop()
	wsInventoryHashInterval := 15 * time.Minute
	var lastWSInventoryHashSent string
	var lastWSInventoryHashAt time.Time

	signalListener := heartbeat.NewSignalListener(client, cfg.Agent.UUID, cfg.Agent.SecretKey, logger, sender.TriggerNow, &wsActive)
	go signalListener.Start(ctx)

	reportFn := func(ctx context.Context, taskID int, req api.TaskStatusRequest) error {
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			_, err := client.ReportTaskStatus(ctx, cfg.Agent.UUID, cfg.Agent.SecretKey, taskID, req)
			if err == nil {
				return nil
			}
			lastErr = err
			logger.Printf("task status report failed for task=%d attempt=%d: %v", taskID, attempt, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
		return lastErr
	}

	executeFn := func(ctx context.Context, cmd api.Command) (queue.ExecutionResult, error) {
		result, err := executeCommand(ctx, *cfg, cmd, logger)
		if err == nil {
			// Rescan installed software immediately after a successful
			// installation so the inventory reflects the change before
			// the next scheduled scan interval.
			invManager.ForceScan()
			if wsActive.Load() {
				select {
				case wsInventoryKickCh <- struct{}{}:
				default:
				}
			}
		}
		return result, err
	}

	var wsStartOnce sync.Once
	var wsClient *wsconn.Client
	announcementTracker := announcement.NewTracker()
	var stateMu sync.Mutex
	restartRequestCh := make(chan string, 1)
	requestRestart := func(reason string) {
		select {
		case restartRequestCh <- reason:
		default:
		}
	}
	parseAnnouncementID := func(v any) (int, bool) {
		switch t := v.(type) {
		case int:
			return t, t > 0
		case int32:
			id := int(t)
			return id, id > 0
		case int64:
			id := int(t)
			return id, id > 0
		case float64:
			id := int(t)
			return id, id > 0
		case string:
			id, err := strconv.Atoi(strings.TrimSpace(t))
			if err != nil || id <= 0 {
				return 0, false
			}
			return id, true
		default:
			return 0, false
		}
	}
	handleAnnouncementPush := func(payload map[string]any) {
		if len(payload) == 0 {
			return
		}
		id, ok := parseAnnouncementID(payload["announcement_id"])
		if !ok {
			logger.Printf("announcement: invalid payload, missing announcement_id")
			return
		}
		title, _ := payload["title"].(string)
		message, _ := payload["message"].(string)
		priority, _ := payload["priority"].(string)
		if strings.TrimSpace(priority) == "" {
			priority = "normal"
		}

		announcementTracker.Add(id, title, message, priority)
		go func(announcementID int, annTitle, annMessage, annPriority string) {
			announcement.ShowMessageBox(annTitle, annMessage, annPriority)
			if wsClient != nil {
				if ok := wsClient.SendEvent(ctx, "agent.announcement.ack", map[string]any{
					"announcement_id": announcementID,
				}); !ok {
					logger.Printf("announcement: failed to send ack for id=%d", announcementID)
				}
			} else {
				logger.Printf("announcement: ws client unavailable, ack skipped for id=%d", announcementID)
			}
			announcementTracker.Remove(announcementID)
		}(id, title, message, priority)
	}
	processPendingAnnouncements := func(raw any) {
		items, ok := raw.([]any)
		if !ok {
			return
		}
		for _, item := range items {
			payload, ok := item.(map[string]any)
			if !ok {
				continue
			}
			handleAnnouncementPush(payload)
		}
	}
	isPayloadForCurrentPlatform := func(payload map[string]any) bool {
		target, _ := payload["platform"].(string)
		target = strings.TrimSpace(strings.ToLower(target))
		if target == "" {
			return true
		}
		return target == strings.ToLower(runtime.GOOS)
	}
	extractSelfUpdateChanges := func(payload map[string]any) map[string]any {
		changes := map[string]any{}
		if payload == nil {
			return changes
		}
		if wrapped, ok := payload["changes"].(map[string]any); ok && len(wrapped) > 0 {
			payload = wrapped
		}
		if v, ok := payload["latest_agent_version"]; ok {
			changes["latest_agent_version"] = v
		}
		if v, ok := payload["agent_download_url"]; ok {
			changes["agent_download_url"] = v
		}
		if v, ok := payload["agent_hash"]; ok {
			changes["agent_hash"] = v
		}
		if v, ok := payload["mode"]; ok {
			changes["mode"] = v
		}
		return changes
	}
	applySelfUpdateChanges := func(changes map[string]any) {
		if len(changes) == 0 {
			return
		}
		if taskQueue.PendingCount() == 0 {
			if err := updater.ApplyIfPending(ctx, *cfg, cfgPath, serviceExe, logger); err != nil {
				if errors.Is(err, updater.ErrUpdateRestart) {
					logger.Println("ws: self-update restart triggered")
					requestRestart("self-update apply")
					return
				}
				logger.Printf("ws: self-update apply failed: %v", err)
			}
		}
		if err := updater.StageIfNeeded(ctx, *cfg, changes, logger); err != nil {
			logger.Printf("ws: self-update stage failed: %v", err)
			return
		}
		if taskQueue.PendingCount() == 0 {
			if err := updater.ApplyIfPending(ctx, *cfg, cfgPath, serviceExe, logger); err != nil {
				if errors.Is(err, updater.ErrUpdateRestart) {
					logger.Println("ws: self-update restart triggered")
					requestRestart("self-update apply")
					return
				}
				logger.Printf("ws: self-update apply failed: %v", err)
			}
		}
	}
	sendWSInventoryHash := func(force bool, reason string) {
		if !wsActive.Load() || wsClient == nil {
			return
		}
		hash := strings.TrimSpace(invManager.GetCurrentHash())
		if hash == "" {
			return
		}
		now := time.Now()
		if !force && strings.EqualFold(hash, lastWSInventoryHashSent) && now.Sub(lastWSInventoryHashAt) < wsInventoryHashInterval {
			return
		}
		if wsClient.SendEvent(ctx, "agent.inventory.hash", map[string]any{"hash": hash}) {
			lastWSInventoryHashSent = hash
			lastWSInventoryHashAt = now
			logger.Printf("ws: inventory hash sent (%s): %s", reason, hash)
		}
	}
	var startWSClient func()
	startWSClient = func() {
		wsStartOnce.Do(func() {
			hostInfo := system.CollectHostInfo()
			wsClient = wsconn.NewClient(wsconn.Config{
				ServerURL:       cfg.Server.URL,
				WSURL:           cfg.WebSocket.URL,
				AgentUUID:       cfg.Agent.UUID,
				SecretKey:       cfg.Agent.SecretKey,
				Version:         cfg.Agent.Version,
				Platform:        runtime.GOOS,
				Arch:            runtime.GOARCH,
				Hostname:        hostInfo.Hostname,
				OSVersion:       hostInfo.OSVersion,
				IPAddress:       hostInfo.IPAddress,
				FullIP:          hostInfo.IPAddresses,
				ReconnectMinSec: cfg.WebSocket.ReconnectMinSec,
				ReconnectMaxSec: cfg.WebSocket.ReconnectMaxSec,
				Callbacks: wsconn.Callbacks{
					OnConnected: func() {
						wsActive.Store(true)
						sender.SetWSActive(true)
						logger.Println("ws: connected, HTTP heartbeat suppressed")
						select {
						case wsInventoryKickCh <- struct{}{}:
						default:
						}
					},
					OnDisconnected: func() {
						wsActive.Store(false)
						sender.SetWSActive(false)
						sender.TriggerNow()
						logger.Println("ws: disconnected, HTTP heartbeat resumed")
					},
					OnSignal: func() {
						sender.TriggerNow()
					},
					OnServerHello: func(payload map[string]any) {
						logger.Printf("ws: server.hello received")
						if serverConfig, ok := payload["config"].(map[string]any); ok {
							applySelfUpdateChanges(serverConfig)
							stateMu.Lock()
							applyServerConfig(serverConfig, logger, invManager, traySup, &storeTrayEnabled, &remoteSupportEnabled, runtimeMgr, cfg, cfgPath, startWSClient)
							stateMu.Unlock()
						}
						processPendingAnnouncements(payload["pending_announcements"])
						commands := parseCommandsFromPayload(payload)
						if len(commands) > 0 {
							stateMu.Lock()
							processCommands(ctx, commands, taskQueue, time.Now().UTC(), *cfg, executeFn, reportFn, logger)
							stateMu.Unlock()
						}
						stateMu.Lock()
						handleRSRequest(ctx, parseRSRequestFromPayload(payload), sessionMgr, &remoteSupportEnabled, logger)
						stateMu.Unlock()
						stateMu.Lock()
						handleRSEnd(ctx, parseRSEndFromPayload(payload), sessionMgr)
						stateMu.Unlock()
					},
					OnServerCommand: func(payload map[string]any) {
						commands := parseCommandsFromPayload(payload)
						if len(commands) == 0 {
							return
						}
						stateMu.Lock()
						processCommands(ctx, commands, taskQueue, time.Now().UTC(), *cfg, executeFn, reportFn, logger)
						stateMu.Unlock()
					},
					OnRSRequest: func(payload map[string]any) {
						stateMu.Lock()
						handleRSRequest(ctx, parseRSRequestFromPayload(payload), sessionMgr, &remoteSupportEnabled, logger)
						stateMu.Unlock()
					},
					OnRSEnd: func(payload map[string]any) {
						stateMu.Lock()
						handleRSEnd(ctx, parseRSEndFromPayload(payload), sessionMgr)
						stateMu.Unlock()
					},
					OnConfigPatch: func(payload map[string]any) {
						changes, ok := payload["changes"].(map[string]any)
						if !ok {
							return
						}
						applySelfUpdateChanges(changes)
						stateMu.Lock()
						applyServerConfig(changes, logger, invManager, traySup, &storeTrayEnabled, &remoteSupportEnabled, runtimeMgr, cfg, cfgPath, startWSClient)
						stateMu.Unlock()
					},
					OnBroadcastRestart: func(payload map[string]any) {
						reason, _ := payload["reason"].(string)
						reason = strings.TrimSpace(reason)
						if reason == "" {
							reason = "ws-broadcast"
						}
						logger.Printf("ws: restart requested by server (%s)", reason)
						requestRestart(reason)
					},
					OnBroadcastSelfUpdate: func(payload map[string]any) {
						if !isPayloadForCurrentPlatform(payload) {
							logger.Printf("ws: self-update broadcast ignored due to platform mismatch")
							return
						}
						changes := extractSelfUpdateChanges(payload)
						if len(changes) == 0 {
							logger.Printf("ws: self-update broadcast ignored due to missing update metadata")
							return
						}
						logger.Printf("ws: self-update broadcast received")
						applySelfUpdateChanges(changes)
					},
					OnInventorySyncRequired: func(payload map[string]any) {
						// Trigger a heartbeat so existing inventory sync flow can run immediately.
						sender.TriggerNow()
					},
					OnAnnouncementPush: func(payload map[string]any) {
						handleAnnouncementPush(payload)
					},
				},
				Logger: logger,
			})
			go wsClient.Run(ctx)
			logger.Println("ws: client started")
		})
	}

	if cfg.WebSocket.Enabled {
		startWSClient()
	}

	logger.Println("service loop started")

	for {
		select {
		case <-ctx.Done():
			logger.Println("service loop stopping")
			time.Sleep(200 * time.Millisecond)
			logger.Println("service loop stopped")
			return nil
		case reason := <-restartRequestCh:
			logger.Printf("service restart requested via ws: %s", reason)
			return updater.ErrUpdateRestart
		case <-wsInventoryKickCh:
			if wsActive.Load() {
				invManager.ForceScan()
				sendWSInventoryHash(true, "connect-or-change")
			}
		case <-wsInventoryTicker.C:
			if wsActive.Load() {
				changed := invManager.ScanIfNeeded()
				if changed {
					sendWSInventoryHash(true, "periodic-scan-changed")
				} else {
					sendWSInventoryHash(false, "periodic-heartbeat")
				}
			}
		case result := <-pollResults:
			if taskQueue.PendingCount() == 0 {
				if err := updater.ApplyIfPending(ctx, *cfg, cfgPath, serviceExe, logger); err != nil {
					if errors.Is(err, updater.ErrUpdateRestart) {
						return err
					}
					logger.Printf("self-update apply failed: %v", err)
				}
			}
			if err := updater.StageIfNeeded(ctx, *cfg, result.Config, logger); err != nil {
				logger.Printf("self-update stage failed: %v", err)
			}

			stateMu.Lock()
			applyServerConfig(result.Config, logger, invManager, traySup, &storeTrayEnabled, &remoteSupportEnabled, runtimeMgr, cfg, cfgPath, startWSClient)
			stateMu.Unlock()
			stateMu.Lock()
			handleRSRequest(ctx, result.RemoteSupportRequest, sessionMgr, &remoteSupportEnabled, logger)
			stateMu.Unlock()
			stateMu.Lock()
			handleRSEnd(ctx, result.RemoteSupportEnd, sessionMgr)
			stateMu.Unlock()
			for _, pending := range result.PendingAnnouncements {
				handleAnnouncementPush(pending)
			}

			// Periodic inventory scan.
			invManager.ScanIfNeeded()

			// Submit inventory if server requests sync.
			if result.InventorySyncRequired {
				submitFn := func(sctx context.Context, payload inventory.SubmitRequest) (*inventory.SubmitResponse, error) {
					raw, err := client.SubmitInventory(sctx, cfg.Agent.UUID, cfg.Agent.SecretKey, payload)
					if err != nil {
						return nil, err
					}
					resp := &inventory.SubmitResponse{
						Status:  fmt.Sprintf("%v", raw["status"]),
						Message: fmt.Sprintf("%v", raw["message"]),
						Changes: make(map[string]int),
					}
					if ch, ok := raw["changes"].(map[string]any); ok {
						for k, v := range ch {
							if f, ok := v.(float64); ok {
								resp.Changes[k] = int(f)
							}
						}
					}
					return resp, nil
				}
				invManager.SyncIfRequested(ctx, true, submitFn)
			}

			stateMu.Lock()
			processCommands(ctx, result.Commands, taskQueue, result.ServerTime, *cfg, executeFn, reportFn, logger)
			stateMu.Unlock()
		}
	}
}

func applyServerConfig(
	serverConfig map[string]any,
	logger *log.Logger,
	invManager *inventory.Manager,
	traySup *traySupervisor,
	storeTrayEnabled *atomic.Bool,
	remoteSupportEnabled *atomic.Bool,
	runtimeMgr *runtimeupdate.Manager,
	cfg *config.Config,
	cfgPath string,
	onWSEnabled func(),
) {
	if serverConfig == nil {
		return
	}
	if v, ok := serverConfig["inventory_scan_interval_min"]; ok {
		if f, ok := v.(float64); ok {
			invManager.SetScanInterval(int(f))
		}
	}
	if v, ok := serverConfig["store_tray_enabled"]; ok {
		if b, ok := v.(bool); ok {
			// Keep enforcing desired tray state on every heartbeat so unexpected tray exits are healed automatically.
			if b {
				traySup.SetEnabled(true)
				if !storeTrayEnabled.Load() {
					logger.Printf("tray supervisor: store_tray_enabled=true")
				}
			} else if storeTrayEnabled.Load() {
				traySup.SetEnabled(false)
				logger.Printf("tray supervisor: store_tray_enabled=false")
			}
			storeTrayEnabled.Store(b)
		}
	}
	if v, ok := serverConfig["remote_support_enabled"]; ok {
		if b, ok := v.(bool); ok {
			prev := remoteSupportEnabled.Load()
			remoteSupportEnabled.Store(b)
			if prev != b {
				logger.Printf("remote support: remote_support_enabled=%t", b)
			}
		}
	}
	if v, ok := serverConfig["websocket_enabled"]; ok {
		if b, ok := v.(bool); ok {
			if b && !cfg.WebSocket.Enabled {
				cfg.WebSocket.Enabled = true
				logger.Printf("ws: enabled via server config")
				// Persist so the next restart starts WS client automatically.
				if err := config.Save(cfgPath, cfg); err != nil {
					logger.Printf("ws: failed to persist websocket.enabled=true: %v", err)
				}
				if onWSEnabled != nil {
					onWSEnabled()
				}
			} else if !b && cfg.WebSocket.Enabled {
				cfg.WebSocket.Enabled = false
				logger.Printf("ws: disabled via server config (restart required to stop active connection)")
				if err := config.Save(cfgPath, cfg); err != nil {
					logger.Printf("ws: failed to persist websocket.enabled=false: %v", err)
				}
			}
		}
	}
	runtimeMgr.UpdateConfig(runtimeupdate.Config{
		BaseURL:     runtimeUpdateBaseURL(cfg.Server.URL),
		IntervalMin: configInt(serverConfig, "runtime_update_interval_min", 60),
		JitterSec:   configInt(serverConfig, "runtime_update_jitter_sec", 300),
	})
}

func processCommands(
	ctx context.Context,
	commands []api.Command,
	taskQueue *queue.TaskQueue,
	serverTime time.Time,
	cfg config.Config,
	executeFn func(context.Context, api.Command) (queue.ExecutionResult, error),
	reportFn func(context.Context, int, api.TaskStatusRequest) error,
	logger *log.Logger,
) {
	if len(commands) > 0 {
		taskQueue.AddCommands(commands)
		logger.Printf("received %d command(s), pending=%d", len(commands), taskQueue.PendingCount())
	}
	for {
		processed := taskQueue.ProcessOne(ctx, serverTime, cfg, executeFn, reportFn)
		if !processed {
			break
		}
	}
}

func handleRSRequest(
	ctx context.Context,
	req *api.RemoteSupportRequest,
	sessionMgr *remotesupport.SessionManager,
	remoteSupportEnabled *atomic.Bool,
	logger *log.Logger,
) {
	if req == nil {
		return
	}
	if sessionMgr != nil && remoteSupportEnabled.Load() {
		go sessionMgr.HandleRequest(ctx, *req)
		return
	}
	if !remoteSupportEnabled.Load() {
		logger.Printf("remote support: request ignored (disabled by server policy)")
	}
}

func handleRSEnd(
	ctx context.Context,
	end *api.RemoteSupportEnd,
	sessionMgr *remotesupport.SessionManager,
) {
	if sessionMgr != nil && end != nil {
		go sessionMgr.HandleEndSignal(ctx, *end)
	}
}

func parseCommandsFromPayload(payload map[string]any) []api.Command {
	if len(payload) == 0 {
		return nil
	}
	commandsRaw := payload["commands"]
	if commandsRaw == nil {
		commandsRaw = payload["pending_commands"]
	}
	if commandsRaw == nil {
		return nil
	}
	b, err := json.Marshal(commandsRaw)
	if err != nil {
		return nil
	}
	var commands []api.Command
	if err := json.Unmarshal(b, &commands); err != nil {
		return nil
	}
	return commands
}

func parseRSRequestFromPayload(payload map[string]any) *api.RemoteSupportRequest {
	if len(payload) == 0 {
		return nil
	}
	raw := payload["remote_support_request"]
	if raw == nil {
		raw = payload["pending_rs_request"]
	}
	if raw == nil {
		raw = payload
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var req api.RemoteSupportRequest
	if err := json.Unmarshal(b, &req); err != nil || req.SessionID <= 0 {
		return nil
	}
	return &req
}

func parseRSEndFromPayload(payload map[string]any) *api.RemoteSupportEnd {
	if len(payload) == 0 {
		return nil
	}
	raw := payload["remote_support_end"]
	if raw == nil {
		raw = payload["pending_rs_end"]
	}
	if raw == nil {
		raw = payload
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var end api.RemoteSupportEnd
	if err := json.Unmarshal(b, &end); err != nil || end.SessionID <= 0 {
		return nil
	}
	return &end
}

func configInt(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return n
		}
	}
	return def
}

func keys(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func runtimeUpdateBaseURL(serverURL string) string {
	return strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/uploads/agent_runtime"
}

func buildIPCHandler(
	client *api.Client,
	cfg *config.Config,
	taskQueue *queue.TaskQueue,
	logger *log.Logger,
	startedAt time.Time,
	sessionMgr *remotesupport.SessionManager,
	remoteSupportEnabled *atomic.Bool,
) ipc.Handler {
	return func(req ipc.Request) ipc.Response {
		switch strings.ToLower(req.Action) {
		case "get_status":
			return ipc.Response{
				Status: "ok",
				Data: map[string]any{
					"service":       "running",
					"started_at":    startedAt.Format(time.RFC3339),
					"pending_tasks": taskQueue.PendingCount(),
					"agent_version": cfg.Agent.Version,
					"agent_uuid":    cfg.Agent.UUID,
				},
			}
		case "get_store":
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			store, err := client.GetStore(ctx, cfg.Agent.UUID, cfg.Agent.SecretKey)
			if err != nil {
				logger.Printf("ipc get_store failed: %v", err)
				return ipc.Response{Status: "error", Message: err.Error()}
			}
			return ipc.Response{Status: "ok", Data: store}
		case "install_from_store":
			if req.AppID <= 0 {
				return ipc.Response{Status: "error", Message: "app_id is required"}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			resp, err := client.RequestStoreInstall(ctx, cfg.Agent.UUID, cfg.Agent.SecretKey, req.AppID)
			if err != nil {
				logger.Printf("ipc install_from_store failed: %v", err)
				return ipc.Response{Status: "error", Message: err.Error()}
			}

			msg := "install request queued"
			queueStatus := "queued"
			if resp != nil && resp.Message != "" {
				msg = resp.Message
			}
			if resp != nil && resp.Status != "" {
				queueStatus = resp.Status
			}
			return ipc.Response{
				Status:  "ok",
				Message: msg,
				Data: map[string]any{
					"queue_status": queueStatus,
				},
			}
		case "remote_support_status":
			enabled := sessionMgr != nil && remoteSupportEnabled != nil && remoteSupportEnabled.Load()
			if !enabled {
				return ipc.Response{
					Status: "ok",
					Data: map[string]any{
						"enabled": false,
						"state":   "disabled",
					},
				}
			}
			helperRunning, helperPID := sessionMgr.HelperStatus()
			return ipc.Response{
				Status: "ok",
				Data: map[string]any{
					"enabled":        true,
					"state":          string(sessionMgr.State()),
					"session_id":     sessionMgr.CurrentSessionID(),
					"helper_running": helperRunning,
					"helper_pid":     helperPID,
				},
			}
		case "remote_support_end":
			if sessionMgr == nil {
				return ipc.Response{Status: "error", Message: "remote support disabled"}
			}
			go sessionMgr.EndSession(context.Background(), "user")
			return ipc.Response{Status: "ok", Message: "session end requested"}
		default:
			return ipc.Response{Status: "error", Message: "unknown action"}
		}
	}
}

func bootstrapAgent(client *api.Client, cfg *config.Config, logger interface{ Printf(string, ...any) }) error {
	if cfg.Agent.UUID == "" {
		u, err := system.GetOrCreateUUID()
		if err != nil {
			return err
		}
		cfg.Agent.UUID = u
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if cfg.Agent.SecretKey == "" {
		info := system.CollectHostInfo()
		resp, err := client.Register(ctx, cfg.Agent.UUID, cfg.Agent.Version, info)
		if err != nil {
			return err
		}
		if resp.SecretKey == "" {
			return errors.New("empty secret_key in register response")
		}
		cfg.Agent.SecretKey = resp.SecretKey
		if err := config.Save(resolveWritableConfigPath(), cfg); err != nil {
			logger.Printf("warning: config not persisted: %v", err)
		}
		logger.Printf("agent registered: %s", cfg.Agent.UUID)
		return nil
	}

	logger.Printf("agent bootstrap reused existing credentials: %s", cfg.Agent.UUID)
	return nil
}

func executeCommand(
	ctx context.Context,
	cfg config.Config,
	cmd api.Command,
	logger interface{ Printf(string, ...any) },
) (queue.ExecutionResult, error) {
	if err := os.MkdirAll(cfg.Download.TempDir, 0o755); err != nil {
		return queue.ExecutionResult{ExitCode: -1}, err
	}
	if logger != nil {
		logger.Printf("task=%d app=%d install start: action=%s version=%s", cmd.TaskID, cmd.AppID, cmd.Action, cmd.AppVersion)
	}

	basePath := filepath.Join(cfg.Download.TempDir, fmt.Sprintf("task_%d_app_%d", cmd.TaskID, cmd.AppID))
	downloadPath := findExistingDownloadPath(basePath)

	downloadURL := cmd.DownloadURL
	if strings.HasPrefix(downloadURL, "/") {
		downloadURL = strings.TrimRight(cfg.Server.URL, "/") + downloadURL
	}

	downloadStarted := time.Now()
	meta, err := downloader.DownloadFileWithMeta(
		ctx,
		downloadURL,
		downloadPath,
		cfg.Download.BandwidthLimitKBs,
		cfg.Agent.UUID,
		cfg.Agent.SecretKey,
	)
	downloadDuration := int(time.Since(downloadStarted).Seconds())
	if err != nil {
		if logger != nil {
			logger.Printf("task=%d app=%d install failed at download: err=%v", cmd.TaskID, cmd.AppID, err)
		}
		return queue.ExecutionResult{ExitCode: -1, DownloadDurationSec: downloadDuration}, fmt.Errorf("download failed: %w", err)
	}
	if logger != nil {
		logger.Printf("task=%d app=%d download completed: bytes=%d file=%s", cmd.TaskID, cmd.AppID, meta.BytesWritten, meta.Filename)
	}

	installPath := downloadPath
	if ext := strings.ToLower(filepath.Ext(meta.Filename)); ext == ".msi" || ext == ".exe" || ext == ".ps1" {
		candidate := basePath + ext
		if candidate != downloadPath {
			if renameErr := os.Rename(downloadPath, candidate); renameErr == nil {
				installPath = candidate
			}
		}
	}

	valid, err := utils.VerifyFileHash(installPath, cmd.FileHash)
	if err != nil {
		if logger != nil {
			logger.Printf("task=%d app=%d install failed at hash verify: err=%v", cmd.TaskID, cmd.AppID, err)
		}
		return queue.ExecutionResult{ExitCode: -1, DownloadDurationSec: downloadDuration}, fmt.Errorf("hash verification failed: %w", err)
	}
	if !valid {
		if logger != nil {
			logger.Printf("task=%d app=%d install failed: hash mismatch", cmd.TaskID, cmd.AppID)
		}
		return queue.ExecutionResult{ExitCode: -1, DownloadDurationSec: downloadDuration}, errors.New("hash mismatch")
	}

	installerType := strings.ToLower(filepath.Ext(installPath))
	if logger != nil {
		logger.Printf("task=%d app=%d installer run: type=%s args=%q", cmd.TaskID, cmd.AppID, installerType, cmd.InstallArgs)
	}
	installStarted := time.Now()
	exitCode, err := installer.Install(installPath, cmd.InstallArgs, cfg.Install.TimeoutSec)
	installDuration := int(time.Since(installStarted).Seconds())
	if err != nil {
		if logger != nil {
			logger.Printf(
				"task=%d app=%d install failed: type=%s exit=%d download_sec=%d install_sec=%d err=%v",
				cmd.TaskID,
				cmd.AppID,
				installerType,
				exitCode,
				downloadDuration,
				installDuration,
				err,
			)
		}
		return queue.ExecutionResult{
			ExitCode:            exitCode,
			DownloadDurationSec: downloadDuration,
			InstallDurationSec:  installDuration,
		}, fmt.Errorf("install failed: %w", err)
	}

	if cfg.Install.EnableAutoCleanup {
		_ = os.Remove(installPath)
	}
	if logger != nil {
		logger.Printf(
			"task=%d app=%d install success: type=%s exit=%d download_sec=%d install_sec=%d",
			cmd.TaskID,
			cmd.AppID,
			installerType,
			exitCode,
			downloadDuration,
			installDuration,
		)
	}

	return queue.ExecutionResult{
		ExitCode:            exitCode,
		InstalledVersion:    cmd.AppVersion,
		DownloadDurationSec: downloadDuration,
		InstallDurationSec:  installDuration,
		Message:             "Installation completed successfully",
	}, nil
}

func findExistingDownloadPath(basePath string) string {
	for _, ext := range []string{".msi", ".exe", ".ps1", ".bin"} {
		candidate := basePath + ext
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return basePath + ".bin"
}

func resolveConfigPath() string {
	if p := os.Getenv("APPCENTER_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\AppCenter\config.yaml`
	}
	return "configs/config.yaml.template"
}

func resolveWritableConfigPath() string {
	if p := os.Getenv("APPCENTER_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\AppCenter\config.yaml`
	}
	return "config.yaml"
}

func logPathOrFallback(path string) string {
	if path == "" {
		return "agent.log"
	}
	return path
}
