package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"appcenter-agent/internal/api"
	"appcenter-agent/internal/config"
)

func defaultConfig() config.Config {
	return config.Config{
		WorkHours: config.WorkHoursConfig{StartUTC: "09:00", EndUTC: "18:00"},
	}
}

func TestProcessOneSuccessSetsAppsChanged(t *testing.T) {
	q := NewTaskQueue(3)
	q.randIntn = func(_ int) int { return 0 }

	q.AddCommands([]api.Command{{TaskID: 10, AppID: 5, AppVersion: "1.2.3", Priority: 1}})

	reported := api.TaskStatusRequest{}
	processed := q.ProcessOne(
		context.Background(),
		time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC),
		defaultConfig(),
		func(context.Context, api.Command) (ExecutionResult, error) {
			return ExecutionResult{InstalledVersion: "1.2.3"}, nil
		},
		func(_ context.Context, _ int, req api.TaskStatusRequest) error {
			reported = req
			return nil
		},
	)

	if !processed {
		t.Fatal("expected one task to be processed")
	}
	if reported.Status != "success" {
		t.Fatalf("status=%s, want success", reported.Status)
	}

	changed, apps := q.ConsumeAppsChanged()
	if !changed {
		t.Fatal("apps_changed should be true after success")
	}
	if len(apps) != 1 || apps[0].AppID != 5 || apps[0].Version != "1.2.3" {
		t.Fatalf("unexpected apps payload: %+v", apps)
	}
}

func TestRetryAndMaxRetries(t *testing.T) {
	q := NewTaskQueue(2)
	q.randIntn = func(_ int) int { return 0 }
	fakeNow := time.Date(2026, 2, 14, 10, 0, 0, 0, time.UTC)
	q.nowFn = func() time.Time { return fakeNow }

	q.AddCommands([]api.Command{{TaskID: 20, AppID: 9, Priority: 1}})

	cfg := defaultConfig()
	execFail := func(context.Context, api.Command) (ExecutionResult, error) {
		return ExecutionResult{ExitCode: 1603}, errors.New("install failed")
	}
	reportNoop := func(context.Context, int, api.TaskStatusRequest) error { return nil }

	if !q.ProcessOne(context.Background(), fakeNow, cfg, execFail, reportNoop) {
		t.Fatal("first failure should process task")
	}
	if q.PendingCount() != 1 {
		t.Fatalf("pending=%d, want 1", q.PendingCount())
	}

	if q.ProcessOne(context.Background(), fakeNow, cfg, execFail, reportNoop) {
		t.Fatal("task should not run before retry delay")
	}

	fakeNow = fakeNow.Add(5 * time.Minute)
	if !q.ProcessOne(context.Background(), fakeNow, cfg, execFail, reportNoop) {
		t.Fatal("second failure should process task")
	}
	if q.PendingCount() != 0 {
		t.Fatalf("pending=%d, want 0 after max retries", q.PendingCount())
	}
}

func TestWorkHoursAndForceUpdate(t *testing.T) {
	cfg := defaultConfig()

	if !isWithinWorkHours(time.Date(2026, 2, 14, 9, 0, 0, 0, time.UTC), cfg.WorkHours.StartUTC, cfg.WorkHours.EndUTC) {
		t.Fatal("09:00 should be within work hours")
	}
	if isWithinWorkHours(time.Date(2026, 2, 14, 22, 0, 0, 0, time.UTC), cfg.WorkHours.StartUTC, cfg.WorkHours.EndUTC) {
		t.Fatal("22:00 should be outside work hours")
	}

	normal := api.Command{TaskID: 1, ForceUpdate: false}
	forced := api.Command{TaskID: 2, ForceUpdate: true}
	outside := time.Date(2026, 2, 14, 22, 0, 0, 0, time.UTC)

	if shouldExecuteNow(normal, outside, cfg.WorkHours.StartUTC, cfg.WorkHours.EndUTC) {
		t.Fatal("normal task should not run outside work hours")
	}
	if !shouldExecuteNow(forced, outside, cfg.WorkHours.StartUTC, cfg.WorkHours.EndUTC) {
		t.Fatal("force update should bypass work hours")
	}
}
