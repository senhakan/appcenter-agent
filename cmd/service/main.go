package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"appcenter-agent/internal/api"
	"appcenter-agent/internal/config"
	"appcenter-agent/internal/heartbeat"
	"appcenter-agent/internal/system"
	"appcenter-agent/pkg/utils"
)

func main() {
	cfgPath := os.Getenv("APPCENTER_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml.template"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, logFile, err := utils.NewLogger(logPathOrFallback(cfg.Logging.File))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	if err := bootstrapAgent(cfg, logger); err != nil {
		logger.Printf("bootstrap failed: %v", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sender := heartbeat.NewSender(api.NewClient(cfg.Server), cfg, logger)
	go sender.Start(ctx)

	logger.Println("service loop started")
	<-ctx.Done()
	time.Sleep(200 * time.Millisecond)
	logger.Println("service loop stopped")
}

func bootstrapAgent(cfg *config.Config, logger interface{ Printf(string, ...any) }) error {
	if cfg.Agent.UUID == "" {
		u, err := system.GetOrCreateUUID()
		if err != nil {
			return err
		}
		cfg.Agent.UUID = u
	}

	client := api.NewClient(cfg.Server)
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
