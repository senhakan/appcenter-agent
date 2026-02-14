package ipc

import "testing"

func TestNewRequest(t *testing.T) {
	req := NewRequest("get_status", 0)
	if req.Action != "get_status" {
		t.Fatalf("action=%s", req.Action)
	}
	if req.Timestamp == "" {
		t.Fatal("timestamp should be set")
	}
}
