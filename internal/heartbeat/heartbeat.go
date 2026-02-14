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

type Sender struct {
	client *api.Client
	cfg    *config.Config
	logger *log.Logger
}

func NewSender(client *api.Client, cfg *config.Config, logger *log.Logger) *Sender {
	return &Sender{client: client, cfg: cfg, logger: logger}
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

	req := api.HeartbeatRequest{
		Hostname:      info.Hostname,
		IPAddress:     info.IPAddress,
		OSUser:        u.Username,
		AgentVersion:  s.cfg.Agent.Version,
		DiskFreeGB:    info.DiskFreeGB,
		CPUUsage:      0,
		RAMUsage:      0,
		CurrentStatus: "Idle",
		AppsChanged:   appsChanged,
		InstalledApps: []api.InstalledApp{},
	}

	resp, err := s.client.Heartbeat(ctx, s.cfg.Agent.UUID, s.cfg.Agent.SecretKey, req)
	if err != nil {
		s.logger.Printf("heartbeat error: %v", err)
		return
	}

	s.logger.Printf("heartbeat ok: status=%s commands=%d", resp.Status, len(resp.Commands))
}
