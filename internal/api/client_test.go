package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"appcenter-agent/internal/config"
)

func TestReportTaskStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/task/12/status" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-Agent-UUID") != "u1" || r.Header.Get("X-Agent-Secret") != "s1" {
			t.Fatalf("missing auth headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TaskStatusResponse{Status: "ok"})
	}))
	defer srv.Close()

	c := NewClient(config.ServerConfig{URL: srv.URL})
	resp, err := c.ReportTaskStatus(context.Background(), "u1", "s1", 12, TaskStatusRequest{Status: "success", Progress: 100, Message: "ok"})
	if err != nil {
		t.Fatalf("ReportTaskStatus error: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status = %s, want ok", resp.Status)
	}
}
