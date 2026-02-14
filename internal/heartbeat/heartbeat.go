package heartbeat

import (
	"context"
	"log"
	"os/user"
	"time"

	"appcenter-agent/internal/api"
	"appcenter-agent/internal/config"
	"appcenter-agent/internal/system"
)

type InstalledAppsProvider interface {
	ConsumeAppsChanged() (bool, []api.InstalledApp)
}

type PollResult struct {
	ServerTime time.Time
	Config     map[string]any
	Commands   []api.Command
}

type Sender struct {
	client            *api.Client
	cfg               *config.Config
	logger            *log.Logger
	resultsCh         chan<- PollResult
	installedProvider InstalledAppsProvider
}

func NewSender(
	client *api.Client,
	cfg *config.Config,
	logger *log.Logger,
	resultsCh chan<- PollResult,
	installedProvider InstalledAppsProvider,
) *Sender {
	return &Sender{
		client:            client,
		cfg:               cfg,
		logger:            logger,
		resultsCh:         resultsCh,
		installedProvider: installedProvider,
	}
}

func (s *Sender) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(s.cfg.Heartbeat.IntervalSec) * time.Second)
	defer ticker.Stop()

	s.sendOnce(ctx, false)

	for {
		select {
		case <-ctx.Done():
			s.logger.Println("heartbeat stopped")
			return
		case <-ticker.C:
			s.sendOnce(ctx, false)
		}
	}
}

func (s *Sender) sendOnce(ctx context.Context, appsChanged bool) {
	u, _ := user.Current()
	info := system.CollectHostInfo()
	osUser := ""
	if u != nil {
		osUser = u.Username
	}

	installedApps := []api.InstalledApp{}
	if s.installedProvider != nil {
		if changed, apps := s.installedProvider.ConsumeAppsChanged(); changed {
			appsChanged = true
			installedApps = apps
		}
	}

	req := api.HeartbeatRequest{
		Hostname:      info.Hostname,
		IPAddress:     info.IPAddress,
		OSUser:        osUser,
		AgentVersion:  s.cfg.Agent.Version,
		DiskFreeGB:    info.DiskFreeGB,
		CPUUsage:      0,
		RAMUsage:      0,
		CurrentStatus: "Idle",
		AppsChanged:   appsChanged,
		InstalledApps: installedApps,
	}

	resp, err := s.client.Heartbeat(ctx, s.cfg.Agent.UUID, s.cfg.Agent.SecretKey, req)
	if err != nil {
		s.logger.Printf("heartbeat error: %v", err)
		return
	}

	s.logger.Printf("heartbeat ok: status=%s commands=%d", resp.Status, len(resp.Commands))
	serverTime := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, resp.ServerTime); err == nil {
		serverTime = parsed.UTC()
	}
	if s.resultsCh != nil {
		select {
		case s.resultsCh <- PollResult{
			ServerTime: serverTime,
			Config:     resp.Config,
			Commands:   resp.Commands,
		}:
		default:
			s.logger.Printf("heartbeat result queue full, dropping %d command(s)", len(resp.Commands))
		}
	}
}
