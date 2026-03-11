package heartbeat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"os/user"
	"runtime"
	"sync/atomic"
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

// RemoteSupportProvider returns current remote support runtime status.
type RemoteSupportProvider interface {
	CurrentRemoteSupportStatus() *api.RemoteSupportStatus
}

type PollResult struct {
	ServerTime            time.Time
	Config                map[string]any
	Commands              []api.Command
	PendingAnnouncements  []map[string]any
	InventorySyncRequired bool
	RemoteSupportRequest  *api.RemoteSupportRequest
	RemoteSupportEnd      *api.RemoteSupportEnd
}

type Sender struct {
	client            *api.Client
	cfg               *config.Config
	logger            *log.Logger
	resultsCh         chan<- PollResult
	installedProvider InstalledAppsProvider
	inventoryProvider InventoryHashProvider
	remoteProvider    RemoteSupportProvider
	triggerCh         chan struct{}

	sysProfileStatePath string
	sysProfileLastSent  time.Time
	sysProfileLastHash  string
	servicesEnabled     bool
	servicesSyncNeeded  bool
	servicesIntervalMin int
	servicesLastSent    time.Time
	servicesLastHash    string
	wsActive            atomic.Bool
}

func NewSender(
	client *api.Client,
	cfg *config.Config,
	logger *log.Logger,
	resultsCh chan<- PollResult,
	installedProvider InstalledAppsProvider,
	inventoryProvider InventoryHashProvider,
	remoteProvider RemoteSupportProvider,
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
		client:              client,
		cfg:                 cfg,
		logger:              logger,
		resultsCh:           resultsCh,
		installedProvider:   installedProvider,
		inventoryProvider:   inventoryProvider,
		remoteProvider:      remoteProvider,
		triggerCh:           make(chan struct{}, 1),
		sysProfileStatePath: statePath,
		sysProfileLastSent:  lastSent,
		sysProfileLastHash:  lastHash,
		servicesEnabled:     false,
		servicesSyncNeeded:  false,
		servicesIntervalMin: 10,
	}
}

func (s *Sender) TriggerNow() {
	select {
	case s.triggerCh <- struct{}{}:
	default:
	}
}

func (s *Sender) SetWSActive(active bool) {
	s.wsActive.Store(active)
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
			if s.wsActive.Load() {
				continue
			}
			s.sendOnce(ctx, false)
		case <-s.triggerCh:
			s.logger.Println("heartbeat triggered by signal")
			s.sendOnce(ctx, false)
			ticker.Reset(time.Duration(s.cfg.Heartbeat.IntervalSec) * time.Second)
		}
	}
}

func hashSystemProfile(p system.SystemProfile) string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hashServices(items []api.ServiceItem) string {
	b, _ := json.Marshal(items)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Sender) maybeAttachServices(req *api.HeartbeatRequest) {
	if !s.servicesEnabled {
		return
	}
	intervalMin := s.servicesIntervalMin
	if intervalMin <= 0 {
		intervalMin = 10
	}
	needCollect := s.servicesSyncNeeded || s.servicesLastHash == "" || s.servicesLastSent.IsZero() ||
		time.Since(s.servicesLastSent) >= time.Duration(intervalMin)*time.Minute

	if !needCollect {
		req.ServicesHash = s.servicesLastHash
		return
	}

	rows, err := system.CollectServices()
	if err != nil {
		s.logger.Printf("service snapshot collect failed: %v", err)
		if s.servicesLastHash != "" {
			req.ServicesHash = s.servicesLastHash
		}
		return
	}
	items := make([]api.ServiceItem, 0, len(rows))
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		items = append(items, api.ServiceItem{
			Name:        r.Name,
			DisplayName: r.DisplayName,
			Status:      r.Status,
			StartupType: r.StartupType,
			PID:         r.PID,
			RunAs:       r.RunAs,
			Description: r.Description,
		})
	}
	hash := hashServices(items)
	req.ServicesHash = hash
	if s.servicesSyncNeeded || hash != s.servicesLastHash {
		req.Services = items
		s.servicesLastHash = hash
		s.servicesLastSent = time.Now().UTC()
	}
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
			Index:   d.Index,
			SizeGB:  d.SizeGB,
			Model:   d.Model,
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
		OSFullName:       p.OSFullName,
		OSVersion:        p.OSVersion,
		BuildNumber:      p.BuildNumber,
		Architecture:     p.Architecture,
		Manufacturer:     p.Manufacturer,
		Model:            p.Model,
		CPUModel:         p.CPUModel,
		CPUCoresPhysical: p.CPUCoresPhysical,
		CPUCoresLogical:  p.CPUCoresLogical,
		TotalMemoryGB:    p.TotalMemoryGB,
		DiskCount:        p.DiskCount,
		Disks:            apiDisks,
		Virtualization:   virt,
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
		FullIP:        info.IPAddresses,
		UptimeSec:     info.UptimeSec,
		OSUser:        osUser,
		OSVersion:     info.OSVersion,
		Platform:      "windows",
		Arch:          runtime.GOARCH,
		Distro:        "windows",
		AgentVersion:  s.cfg.Agent.Version,
		CPUModel:      info.CPUModel,
		RAMGB:         info.RAMGB,
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
			Username:     s.Username,
			SessionType:  s.SessionType,
			SessionState: s.SessionState,
			LogonID:      s.LogonID,
		})
	}

	s.maybeAttachSystemProfile(&req)
	s.maybeAttachServices(&req)
	if s.remoteProvider != nil {
		req.RemoteSupport = s.remoteProvider.CurrentRemoteSupportStatus()
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

	inventorySyncRequired := false
	if resp.Config != nil {
		if v, ok := resp.Config["inventory_sync_required"]; ok {
			if b, ok := v.(bool); ok {
				inventorySyncRequired = b
			}
		}
		if v, ok := resp.Config["service_monitoring_enabled"]; ok {
			if b, ok := v.(bool); ok {
				s.servicesEnabled = b
			}
		}
		if v, ok := resp.Config["services_sync_required"]; ok {
			if b, ok := v.(bool); ok {
				s.servicesSyncNeeded = b
			}
		}
		if v, ok := resp.Config["inventory_scan_interval_min"]; ok {
			switch t := v.(type) {
			case float64:
				if int(t) > 0 {
					s.servicesIntervalMin = int(t)
				}
			case int:
				if t > 0 {
					s.servicesIntervalMin = t
				}
			}
		}
	}

	if s.resultsCh != nil {
		select {
		case s.resultsCh <- PollResult{
			ServerTime:            serverTime,
			Config:                resp.Config,
			Commands:              resp.Commands,
			PendingAnnouncements:  resp.PendingAnnouncements,
			InventorySyncRequired: inventorySyncRequired,
			RemoteSupportRequest:  resp.RemoteSupportRequest,
			RemoteSupportEnd:      resp.RemoteSupportEnd,
		}:
		default:
			s.logger.Printf("heartbeat result queue full, dropping %d command(s)", len(resp.Commands))
		}
	}
}
