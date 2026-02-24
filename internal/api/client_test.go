package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

		// Ensure we can send explicit `exit_code=0` without it being dropped.
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := payload["exit_code"]; !ok {
			t.Fatalf("exit_code missing in request payload: %#v", payload)
		}
		if got, ok := payload["exit_code"].(float64); !ok || int(got) != 0 {
			t.Fatalf("exit_code=%v, want 0", payload["exit_code"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(TaskStatusResponse{Status: "ok"})
	}))
	defer srv.Close()

	zero := 0
	c := NewClient(config.ServerConfig{URL: srv.URL})
	resp, err := c.ReportTaskStatus(context.Background(), "u1", "s1", 12, TaskStatusRequest{Status: "success", Progress: 100, Message: "ok", ExitCode: &zero})
	if err != nil {
		t.Fatalf("ReportTaskStatus error: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("status = %s, want ok", resp.Status)
	}
}

func TestGetStore(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/store" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("X-Agent-UUID") != "u1" || r.Header.Get("X-Agent-Secret") != "s1" {
			t.Fatalf("missing auth headers")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StoreResponse{
			Apps: []StoreApp{{ID: 5, DisplayName: "7-Zip", Version: "23.01"}},
		})
	}))
	defer srv.Close()

	c := NewClient(config.ServerConfig{URL: srv.URL})
	resp, err := c.GetStore(context.Background(), "u1", "s1")
	if err != nil {
		t.Fatalf("GetStore error: %v", err)
	}
	if len(resp.Apps) != 1 || resp.Apps[0].ID != 5 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestReportTaskStatus_HTTPErrorIncludesDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "error",
			"detail": "bad task payload",
		})
	}))
	defer srv.Close()

	zero := 0
	c := NewClient(config.ServerConfig{URL: srv.URL})
	_, err := c.ReportTaskStatus(context.Background(), "u1", "s1", 12, TaskStatusRequest{
		Status:   "success",
		Progress: 100,
		Message:  "ok",
		ExitCode: &zero,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "bad task payload") {
		t.Fatalf("err = %q, want detail", err.Error())
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %q, want status code", err.Error())
	}
}

func TestGetStore_HTTPErrorIncludesBodySnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	c := NewClient(config.ServerConfig{URL: srv.URL})
	_, err := c.GetStore(context.Background(), "u1", "s1")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %q, want status code", err.Error())
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %q, want body snippet", err.Error())
	}
}
