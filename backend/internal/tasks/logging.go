package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

type jobLogContextKey struct{}

func protectTask[T any](run func() (T, error)) (result T, err error) {
	defer func() {
		if recover() != nil {
			err = logbook.ErrTaskPanic
		}
	}()
	return run()
}

func protectTaskError(run func() error) error {
	_, err := protectTask(func() (struct{}, error) { return struct{}{}, run() })
	return err
}

func (m *Manager) logExecution(ctx context.Context, job *jobs.Job, next func(context.Context, *jobs.Job) ([]byte, error)) ([]byte, error) {
	entry := logbook.Entry{TaskID: job.ID.String(), TaskType: job.Type, Attempt: job.Attempt}
	if job.Type == "source_crawl" {
		var request sources.CrawlRequest
		if json.Unmarshal(job.Args, &request) == nil {
			entry.Source = request.Source
		}
	} else if job.Type == "source_probe" {
		var request sourceProbeArgs
		if json.Unmarshal(job.Args, &request) == nil {
			entry.Source = request.Source
		}
	} else if job.Type == "actress_crawl" {
		entry.Source = "fanza"
	} else if strings.HasSuffix(job.Type, "_crawl") {
		entry.Source = strings.TrimSuffix(job.Type, "_crawl")
	}
	ctx = context.WithValue(ctx, jobLogContextKey{}, entry)
	quietRetry := job.Type == "metadata_source_retry"
	if job.Type != "maintenance_tick" && job.Type != "metadata_backfill_tick" && !quietRetry {
		started := entry
		started.Level, started.Event = "info", "task_started"
		m.logs.Record(ctx, started)
	}
	result, err := next(ctx, job)
	if err != nil {
		var failure *sources.SourceFailure
		var deferred *sources.DeferredError
		if errors.As(err, &deferred) || errors.As(err, &failure) && !failure.Report {
			return result, err
		}
		entry.Level, entry.Event = "error", "task_failed"
		logbook.ApplyFailure(&entry, err)
		m.logs.Record(ctx, entry)
	} else if job.Type != "maintenance_tick" && job.Type != "metadata_backfill_tick" && !quietRetry {
		entry.Level, entry.Event = "info", "task_completed"
		m.logs.Record(ctx, entry)
	}
	return result, err
}

func (m *Manager) RecordSourceFailure(ctx context.Context, source string, err error) {
	var deferred *sources.DeferredError
	var failure *sources.SourceFailure
	if errors.As(err, &deferred) || errors.As(err, &failure) && !failure.Report {
		return
	}
	entry, _ := ctx.Value(jobLogContextKey{}).(logbook.Entry)
	entry.Level, entry.Event, entry.Source = "warn", "source_failed", source
	logbook.ApplyFailure(&entry, err)
	m.logs.Record(ctx, entry)
}

func (m *Manager) CleanLogs(ctx context.Context) error {
	return m.logs.Clean(ctx, time.Now().UTC().Add(-30*24*time.Hour))
}
