package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

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
	go sender.Start(ctx)

	signalListener := heartbeat.NewSignalListener(client, cfg.Agent.UUID, cfg.Agent.SecretKey, logger, sender.TriggerNow)
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
		result, err := executeCommand(ctx, *cfg, cmd)
		if err == nil {
			// Rescan installed software immediately after a successful
			// installation so the inventory reflects the change before
			// the next scheduled scan interval.
			invManager.ForceScan()
		}
		return result, err
	}

	logger.Println("service loop started")

	for {
		select {
		case <-ctx.Done():
			logger.Println("service loop stopping")
			time.Sleep(200 * time.Millisecond)
			logger.Println("service loop stopped")
			return nil
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

			// Update inventory scan interval from server config.
			if result.Config != nil {
				if v, ok := result.Config["inventory_scan_interval_min"]; ok {
					if f, ok := v.(float64); ok {
						invManager.SetScanInterval(int(f))
					}
				}
				if v, ok := result.Config["store_tray_enabled"]; ok {
					if b, ok := v.(bool); ok {
						// Keep enforcing desired tray state on every heartbeat so
						// unexpected tray exits are healed automatically.
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
				if v, ok := result.Config["remote_support_enabled"]; ok {
					if b, ok := v.(bool); ok {
						prev := remoteSupportEnabled.Load()
						remoteSupportEnabled.Store(b)
						if prev != b {
							logger.Printf("remote support: remote_support_enabled=%t", b)
						}
					}
				}
				runtimeMgr.UpdateConfig(runtimeupdate.Config{
					BaseURL:     runtimeUpdateBaseURL(cfg.Server.URL),
					IntervalMin: configInt(result.Config, "runtime_update_interval_min", 60),
					JitterSec:   configInt(result.Config, "runtime_update_jitter_sec", 300),
				})
			}

			if sessionMgr != nil && result.RemoteSupportRequest != nil && remoteSupportEnabled.Load() {
				go sessionMgr.HandleRequest(ctx, *result.RemoteSupportRequest)
			} else if result.RemoteSupportRequest != nil && !remoteSupportEnabled.Load() {
				logger.Printf("remote support: request ignored (disabled by server policy)")
			}
			if sessionMgr != nil && result.RemoteSupportEnd != nil {
				go sessionMgr.HandleEndSignal(ctx, *result.RemoteSupportEnd)
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

			taskQueue.AddCommands(result.Commands)
			if len(result.Commands) > 0 {
				logger.Printf("received %d command(s), pending=%d", len(result.Commands), taskQueue.PendingCount())
			}

			for {
				processed := taskQueue.ProcessOne(ctx, result.ServerTime, *cfg, executeFn, reportFn)
				if !processed {
					break
				}
			}
		}
	}
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

func executeCommand(ctx context.Context, cfg config.Config, cmd api.Command) (queue.ExecutionResult, error) {
	if err := os.MkdirAll(cfg.Download.TempDir, 0o755); err != nil {
		return queue.ExecutionResult{ExitCode: -1}, err
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
		return queue.ExecutionResult{ExitCode: -1, DownloadDurationSec: downloadDuration}, fmt.Errorf("download failed: %w", err)
	}

	installPath := downloadPath
	if ext := strings.ToLower(filepath.Ext(meta.Filename)); ext == ".msi" || ext == ".exe" {
		candidate := basePath + ext
		if candidate != downloadPath {
			if renameErr := os.Rename(downloadPath, candidate); renameErr == nil {
				installPath = candidate
			}
		}
	}

	valid, err := utils.VerifyFileHash(installPath, cmd.FileHash)
	if err != nil {
		return queue.ExecutionResult{ExitCode: -1, DownloadDurationSec: downloadDuration}, fmt.Errorf("hash verification failed: %w", err)
	}
	if !valid {
		return queue.ExecutionResult{ExitCode: -1, DownloadDurationSec: downloadDuration}, errors.New("hash mismatch")
	}

	installStarted := time.Now()
	exitCode, err := installer.Install(installPath, cmd.InstallArgs, cfg.Install.TimeoutSec)
	installDuration := int(time.Since(installStarted).Seconds())
	if err != nil {
		return queue.ExecutionResult{
			ExitCode:            exitCode,
			DownloadDurationSec: downloadDuration,
			InstallDurationSec:  installDuration,
		}, fmt.Errorf("install failed: %w", err)
	}

	if cfg.Install.EnableAutoCleanup {
		_ = os.Remove(installPath)
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
	for _, ext := range []string{".msi", ".exe", ".bin"} {
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
