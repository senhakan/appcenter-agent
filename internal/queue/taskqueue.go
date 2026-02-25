package queue

import (
	"context"
	"math/rand"
	"sort"
	"sync"
	"time"

	"appcenter-agent/internal/api"
	"appcenter-agent/internal/config"
)

type ExecutionResult struct {
	ExitCode            int
	InstalledVersion    string
	DownloadDurationSec int
	InstallDurationSec  int
	Message             string
}

type ExecuteFunc func(context.Context, api.Command) (ExecutionResult, error)
type ReportFunc func(context.Context, int, api.TaskStatusRequest) error

type RetryInfo struct {
	Count       int
	NextRetryAt time.Time
}

type queuedTask struct {
	Command api.Command
}

type TaskQueue struct {
	mu sync.Mutex

	tasks       map[int]queuedTask
	retries     map[int]*RetryInfo
	maxRetries  int
	installed   map[int]string
	appsChanged bool

	nowFn    func() time.Time
	randIntn func(int) int
}

func NewTaskQueue(maxRetries int) *TaskQueue {
	if maxRetries <= 0 {
		maxRetries = 3
	}
	return &TaskQueue{
		tasks:      make(map[int]queuedTask),
		retries:    make(map[int]*RetryInfo),
		maxRetries: maxRetries,
		installed:  make(map[int]string),
		nowFn:      time.Now,
		randIntn:   rand.Intn,
	}
}

func (q *TaskQueue) AddCommands(commands []api.Command) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, c := range commands {
		if c.TaskID == 0 {
			continue
		}
		if _, exists := q.tasks[c.TaskID]; exists {
			continue
		}
		q.tasks[c.TaskID] = queuedTask{Command: c}
	}
}

func (q *TaskQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

func (q *TaskQueue) ConsumeAppsChanged() (bool, []api.InstalledApp) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.appsChanged {
		return false, []api.InstalledApp{}
	}
	q.appsChanged = false

	appIDs := make([]int, 0, len(q.installed))
	for id := range q.installed {
		appIDs = append(appIDs, id)
	}
	sort.Ints(appIDs)

	apps := make([]api.InstalledApp, 0, len(appIDs))
	for _, id := range appIDs {
		apps = append(apps, api.InstalledApp{AppID: id, Version: q.installed[id]})
	}
	return true, apps
}

func (q *TaskQueue) ProcessOne(
	ctx context.Context,
	serverTime time.Time,
	_ config.Config,
	execute ExecuteFunc,
	report ReportFunc,
) bool {
	task, ok := q.nextRunnable(serverTime.UTC())
	if !ok {
		return false
	}

	if !task.ForceUpdate {
		jitter := q.randIntn(301)
		if jitter > 0 {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Duration(jitter) * time.Second):
			}
		}
	}

	result, err := execute(ctx, task)
	if err != nil {
		q.handleFailure(task.TaskID)
		exitCode := result.ExitCode
		_ = report(ctx, task.TaskID, api.TaskStatusRequest{
			Status:   "failed",
			Progress: 0,
			Message:  err.Error(),
			ExitCode: &exitCode,
			Error:    err.Error(),
		})
		return true
	}

	if result.Message == "" {
		result.Message = "Installation completed successfully"
	}

	exitCode := result.ExitCode
	_ = report(ctx, task.TaskID, api.TaskStatusRequest{
		Status:              "success",
		Progress:            100,
		Message:             result.Message,
		ExitCode:            &exitCode,
		InstalledVersion:    result.InstalledVersion,
		DownloadDurationSec: result.DownloadDurationSec,
		InstallDurationSec:  result.InstallDurationSec,
	})

	q.handleSuccess(task)
	return true
}

func (q *TaskQueue) nextRunnable(serverTime time.Time) (api.Command, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.tasks) == 0 {
		return api.Command{}, false
	}

	candidates := make([]api.Command, 0, len(q.tasks))
	for _, t := range q.tasks {
		if retry, exists := q.retries[t.Command.TaskID]; exists {
			if q.nowFn().Before(retry.NextRetryAt) {
				continue
			}
		}
		if !shouldExecuteNow(t.Command) {
			continue
		}
		candidates = append(candidates, t.Command)
	}

	if len(candidates) == 0 {
		return api.Command{}, false
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].TaskID < candidates[j].TaskID
		}
		return candidates[i].Priority < candidates[j].Priority
	})

	return candidates[0], true
}

func (q *TaskQueue) handleFailure(taskID int) {
	q.mu.Lock()
	defer q.mu.Unlock()

	retry, exists := q.retries[taskID]
	if !exists {
		retry = &RetryInfo{}
		q.retries[taskID] = retry
	}
	retry.Count++

	if retry.Count >= q.maxRetries {
		delete(q.retries, taskID)
		delete(q.tasks, taskID)
		return
	}

	retryDelay := time.Duration(retry.Count*5) * time.Minute
	retry.NextRetryAt = q.nowFn().Add(retryDelay)
}

func (q *TaskQueue) handleSuccess(task api.Command) {
	q.mu.Lock()
	defer q.mu.Unlock()

	delete(q.tasks, task.TaskID)
	delete(q.retries, task.TaskID)

	if task.AppID > 0 {
		version := task.AppVersion
		if version == "" {
			version = "unknown"
		}
		q.installed[task.AppID] = version
		q.appsChanged = true
	}
}

func shouldExecuteNow(_ api.Command) bool {
	return true
}
