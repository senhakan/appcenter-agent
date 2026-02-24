package ipc

import "time"

const PipeName = `\\.\pipe\AppCenterIPC`

type Request struct {
	Action    string `json:"action"`
	AppID     int    `json:"app_id,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Data      any    `json:"data,omitempty"`
}

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

type Handler func(Request) Response

type Server interface {
	Close() error
}

func NewRequest(action string, appID int) Request {
	return Request{
		Action:    action,
		AppID:     appID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
