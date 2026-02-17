package heartbeat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// InventoryHashProvider returns the current inventory hash for inclusion in heartbeat.
type InventoryHashProvider interface {
	GetCurrentHash() string
}

type PollResult struct {
	ServerTime            time.Time
	Config                map[string]any
	Commands              []api.Command
	InventorySyncRequired bool
}

type Sender struct {
	client            *api.Client
	cfg               *config.Config
	logger            *log.Logger
	resultsCh         chan<- PollResult
	installedProvider InstalledAppsProvider
	inventoryProvider InventoryHashProvider

	sysProfileStatePath string
	sysProfileLastSent  time.Time
	sysProfileLastHash  string
}

func NewSender(
	client *api.Client,
	cfg *config.Config,
	logger *log.Logger,
	resultsCh chan<- PollResult,
	installedProvider InstalledAppsProvider,
	inventoryProvider InventoryHashProvider,
) *Sender {
	statePath := system.DefaultSystemProfileStatePath()
	lastSent := time.Time{}
	lastHash := ""
	if st, err := system.LoadSystemProfileState(statePath); err == nil {
		if t, ok := system.ParseUTC(st.LastSentAtUTC); ok {
			lastSent = t
		}
		lastHash = st.LastHash
	}
	return &Sender{
		client:            client,
		cfg:               cfg,
		logger:            logger,
		resultsCh:         resultsCh,
		installedProvider: installedProvider,
		inventoryProvider: inventoryProvider,
		sysProfileStatePath: statePath,
		sysProfileLastSent:  lastSent,
		sysProfileLastHash:  lastHash,
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

func hashSystemProfile(p system.SystemProfile) string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Sender) maybeAttachSystemProfile(req *api.HeartbeatRequest) {
	intervalMin := s.cfg.SystemProfile.ReportIntervalMin
	if intervalMin <= 0 {
		return
	}
	now := time.Now().UTC()
	if !s.sysProfileLastSent.IsZero() && now.Sub(s.sysProfileLastSent) < time.Duration(intervalMin)*time.Minute {
		return
	}

	p, err := system.CollectSystemProfile()
	if err != nil || p == nil {
		return
	}

	// Convert to API struct.
	apiDisks := make([]api.SystemDisk, 0, len(p.Disks))
	for _, d := range p.Disks {
		apiDisks = append(apiDisks, api.SystemDisk{
			Index:  d.Index,
			SizeGB: d.SizeGB,
			Model:  d.Model,
			BusType: d.BusType,
		})
	}
	var virt *api.VirtualizationInfo
	if p.Virtualization != nil {
		virt = &api.VirtualizationInfo{
			IsVirtual: p.Virtualization.IsVirtual,
			Vendor:    p.Virtualization.Vendor,
			Model:     p.Virtualization.Model,
		}
	}
	req.SystemProfile = &api.SystemProfile{
		OSFullName:         p.OSFullName,
		OSVersion:          p.OSVersion,
		BuildNumber:        p.BuildNumber,
		Architecture:       p.Architecture,
		Manufacturer:       p.Manufacturer,
		Model:              p.Model,
		CPUModel:           p.CPUModel,
		CPUCoresPhysical:   p.CPUCoresPhysical,
		CPUCoresLogical:    p.CPUCoresLogical,
		TotalMemoryGB:      p.TotalMemoryGB,
		DiskCount:          p.DiskCount,
		Disks:              apiDisks,
		Virtualization:     virt,
	}

	h := hashSystemProfile(*p)
	s.sysProfileLastSent = now
	s.sysProfileLastHash = h
	_ = system.SaveSystemProfileState(s.sysProfileStatePath, system.SystemProfileState{
		LastSentAtUTC: now.Format(time.RFC3339),
		LastHash:      h,
	})
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

	if s.inventoryProvider != nil {
		req.InventoryHash = s.inventoryProvider.GetCurrentHash()
	}

	// Logged-in sessions are optional; server handles missing field.
	sessions := system.GetLoggedInSessions()
	req.LoggedInSessions = make([]api.LoggedInSession, 0, len(sessions))
	for _, s := range sessions {
		req.LoggedInSessions = append(req.LoggedInSessions, api.LoggedInSession{
			Username:    s.Username,
			SessionType: s.SessionType,
			LogonID:     s.LogonID,
		})
	}

	s.maybeAttachSystemProfile(&req)

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

	inventorySyncRequired := false
	if resp.Config != nil {
		if v, ok := resp.Config["inventory_sync_required"]; ok {
			if b, ok := v.(bool); ok {
				inventorySyncRequired = b
			}
		}
	}

	if s.resultsCh != nil {
		select {
		case s.resultsCh <- PollResult{
			ServerTime:            serverTime,
			Config:                resp.Config,
			Commands:              resp.Commands,
			InventorySyncRequired: inventorySyncRequired,
		}:
		default:
			s.logger.Printf("heartbeat result queue full, dropping %d command(s)", len(resp.Commands))
		}
	}
}
