package remotesupport

import (
	"context"
	"log"
	"sync"

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

	approvalTimeoutSec int
}

func NewSessionManager(
	client *api.Client,
	agentUUID, secret string,
	approvalTimeoutSec int,
	logger *log.Logger,
) *SessionManager {
	if approvalTimeoutSec <= 0 {
		approvalTimeoutSec = 120
	}
	sm := &SessionManager{
		state:              StateIdle,
		client:             client,
		agentUUID:          agentUUID,
		secret:             secret,
		approvalTimeoutSec: approvalTimeoutSec,
		logger:             logger,
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
	return false, 0
}

func (sm *SessionManager) CurrentRemoteSupportStatus() *api.RemoteSupportStatus {
	return &api.RemoteSupportStatus{
		State:         string(sm.State()),
		SessionID:     sm.CurrentSessionID(),
		HelperRunning: false,
		HelperPID:     0,
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

	approved, err := ShowApprovalDialogFromService(req.AdminName, req.Reason, sm.approvalTimeoutSec)
	if err != nil {
		sm.logger.Printf("remote support: approval dialog failed: %v", err)
		sm.reset()
		return
	}

	_, err = sm.client.ApproveRemoteSession(ctx, sm.agentUUID, sm.secret, req.SessionID, approved)
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

	sm.mu.Lock()
	sm.state = StateConnecting
	sm.mu.Unlock()

	// VNC lifecycle is managed externally (UltraVNC service). Agent only reports readiness.
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
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.state = StateIdle
	sm.session = 0
}
