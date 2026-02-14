package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"appcenter-agent/internal/api"
	"appcenter-agent/internal/config"
	"appcenter-agent/internal/downloader"
	"appcenter-agent/internal/heartbeat"
	"appcenter-agent/internal/installer"
	"appcenter-agent/internal/ipc"
	"appcenter-agent/internal/queue"
	"appcenter-agent/internal/system"
	"appcenter-agent/internal/updater"
	"appcenter-agent/pkg/utils"
)

const serviceName = "AppCenterAgent"

func runAgent(ctx context.Context, cfgPath string) error {
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
		return fmt.Errorf("bootstrap failed: %w", err)
	}

	taskQueue := queue.NewTaskQueue(3)
	pollResults := make(chan heartbeat.PollResult, 8)
	serviceStarted := time.Now().UTC()

	pipeServer, pipeErr := ipc.StartPipeServer(buildIPCHandler(client, cfg, taskQueue, logger, serviceStarted))
	if pipeErr != nil {
		logger.Printf("named pipe server not started: %v", pipeErr)
	} else {
		defer pipeServer.Close()
		logger.Printf("named pipe server started: %s", ipc.PipeName)
	}

	sender := heartbeat.NewSender(client, cfg, logger, pollResults, taskQueue)
	go sender.Start(ctx)

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
		return executeCommand(ctx, *cfg, cmd)
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
			if err := updater.StageIfNeeded(ctx, *cfg, result.Config, logger); err != nil {
				logger.Printf("self-update stage failed: %v", err)
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

func buildIPCHandler(
	client *api.Client,
	cfg *config.Config,
	taskQueue *queue.TaskQueue,
	logger *log.Logger,
	startedAt time.Time,
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
			return ipc.Response{
				Status:  "error",
				Message: "install_from_store requires server-side deployment flow in current version",
			}
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
		ExitCode:            0,
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
	return "config.yaml"
}

func logPathOrFallback(path string) string {
	if path == "" {
		return "agent.log"
	}
	return path
}
