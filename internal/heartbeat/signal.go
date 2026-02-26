package heartbeat

import (
	"context"
	"log"
	"time"

	"appcenter-agent/internal/api"
)

const (
	signalPollTimeoutSec = 55
	signalBackoffMin     = 2 * time.Second
	signalBackoffMax     = 60 * time.Second
)

type SignalListener struct {
	client    *api.Client
	agentUUID string
	secretKey string
	logger    *log.Logger
	onSignal  func()
}

func NewSignalListener(
	client *api.Client,
	agentUUID string,
	secretKey string,
	logger *log.Logger,
	onSignal func(),
) *SignalListener {
	return &SignalListener{
		client:    client,
		agentUUID: agentUUID,
		secretKey: secretKey,
		logger:    logger,
		onSignal:  onSignal,
	}
}

func (sl *SignalListener) Start(ctx context.Context) {
	backoff := signalBackoffMin
	for {
		if ctx.Err() != nil {
			sl.logger.Println("signal listener stopped")
			return
		}

		resp, err := sl.client.WaitForSignal(ctx, sl.agentUUID, sl.secretKey, signalPollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				sl.logger.Println("signal listener stopped")
				return
			}
			sl.logger.Printf("signal poll error (retry in %v): %v", backoff, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > signalBackoffMax {
				backoff = signalBackoffMax
			}
			continue
		}

		// Reset backoff on successful response.
		backoff = signalBackoffMin

		if resp.Status == "signal" {
			sl.logger.Println("signal received: triggering immediate heartbeat")
			if sl.onSignal != nil {
				sl.onSignal()
			}
		}
		// For "timeout" status, just loop and re-open the long-poll immediately.
	}
}
