package remotesupport

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"appcenter-agent/internal/api"
)

type SessionState string

const (
	StateIdle       SessionState = "idle"
	StatePending    SessionState = "pending_approval"
	StateApproved   SessionState = "approved"
	StateConnecting SessionState = "connecting"
	StateActive     SessionState = "active"
)

type SessionManager struct {
	mu      sync.Mutex
	state   SessionState
	session int

	client    *api.Client
	agentUUID string
	secret    string
	logger    *log.Logger
	vnc       *VNCServer

	approvalTimeoutSec int
	helperPort         int
}

const (
	defaultApprovalTimeoutSec = 30
	defaultHelperPort         = 5900
	warmupPassword            = "warmup01"
)

func NewSessionManager(
	client *api.Client,
	agentUUID, secret string,
	approvalTimeoutSec int,
	logger *log.Logger,
) *SessionManager {
	if approvalTimeoutSec <= 0 {
		approvalTimeoutSec = defaultApprovalTimeoutSec
	}
	sm := &SessionManager{
		state:              StateIdle,
		client:             client,
		agentUUID:          agentUUID,
		secret:             secret,
		approvalTimeoutSec: approvalTimeoutSec,
		logger:             logger,
		vnc:                NewVNCServer(logger),
		helperPort:         defaultHelperPort,
	}
	return sm
}

func (sm *SessionManager) State() SessionState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.state
}

func (sm *SessionManager) CurrentSessionID() int {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.session
}

func (sm *SessionManager) HelperStatus() (bool, int) {
	return sm.vnc.Status()
}

func (sm *SessionManager) CurrentRemoteSupportStatus() *api.RemoteSupportStatus {
	running, pid := sm.HelperStatus()
	return &api.RemoteSupportStatus{
		State:         string(sm.State()),
		SessionID:     sm.CurrentSessionID(),
		HelperRunning: running,
		HelperPID:     pid,
	}
}

func (sm *SessionManager) HandleRequest(ctx context.Context, req api.RemoteSupportRequest) {
	sm.mu.Lock()
	if sm.state != StateIdle {
		sm.mu.Unlock()
		return
	}
	sm.state = StatePending
	sm.session = req.SessionID
	sm.mu.Unlock()

	// Pre-start helper while waiting for user approval to minimize post-approval latency.
	if err := sm.vnc.Start(warmupPassword, sm.helperPort); err != nil {
		sm.logger.Printf("remote support: helper pre-start failed: %v", err)
		sm.reset()
		return
	}
	if !sm.vnc.WaitListening(sm.helperPort, 8*time.Second) {
		sm.logger.Printf("remote support: helper pre-start listen timeout on %d", sm.helperPort)
		sm.reset()
		return
	}

	approved, err := ShowApprovalDialogFromService(req.AdminName, req.Reason, sm.approvalTimeoutSec)
	if err != nil {
		sm.logger.Printf("remote support: approval dialog failed: %v", err)
		sm.reset()
		return
	}

	approveResp, err := sm.client.ApproveRemoteSession(ctx, sm.agentUUID, sm.secret, req.SessionID, approved)
	if err != nil {
		sm.logger.Printf("remote support: approve report failed: %v", err)
		sm.reset()
		return
	}
	if !approved {
		sm.logger.Printf("remote support: session %d rejected", req.SessionID)
		sm.reset()
		return
	}

	sm.mu.Lock()
	sm.state = StateApproved
	sm.mu.Unlock()

	vncPassword := ""
	targetPort := sm.helperPort
	if approveResp != nil {
		vncPassword = strings.TrimSpace(approveResp.VNCPassword)
	}
	if vncPassword == "" {
		sm.logger.Printf("remote support: missing vnc password for session %d", req.SessionID)
		sm.reset()
		return
	}

	// Re-start helper with session password before reporting ready.
	if err := sm.vnc.Start(vncPassword, targetPort); err != nil {
		sm.logger.Printf("remote support: helper start with session password failed: %v", err)
		sm.reset()
		return
	}
	if !sm.vnc.WaitListening(targetPort, 10*time.Second) {
		sm.logger.Printf("remote support: helper listen timeout on %d after approval", targetPort)
		sm.reset()
		return
	}

	sm.mu.Lock()
	sm.state = StateConnecting
	sm.mu.Unlock()

	if err := sm.client.ReportRemoteReady(ctx, sm.agentUUID, sm.secret, req.SessionID); err != nil {
		sm.logger.Printf("remote support: ready report failed: %v", err)
		sm.reset()
		return
	}

	sm.mu.Lock()
	sm.state = StateActive
	sm.mu.Unlock()
	sm.logger.Printf("remote support: session %d active", req.SessionID)
}

func (sm *SessionManager) HandleEndSignal(ctx context.Context, end api.RemoteSupportEnd) {
	sm.mu.Lock()
	if sm.session != end.SessionID {
		sm.mu.Unlock()
		return
	}
	sm.mu.Unlock()
	sm.EndSession(ctx, "admin")
}

func (sm *SessionManager) EndSession(ctx context.Context, endedBy string) {
	sm.mu.Lock()
	sessionID := sm.session
	sm.mu.Unlock()

	if sessionID == 0 {
		return
	}
	if err := sm.client.ReportRemoteEnded(ctx, sm.agentUUID, sm.secret, sessionID, endedBy); err != nil {
		sm.logger.Printf("remote support: ended report failed: %v", err)
	}
	sm.reset()
}

func (sm *SessionManager) reset() {
	sm.vnc.Stop()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state = StateIdle
	sm.session = 0
}
