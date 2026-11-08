package tasks

import (
	"context"
	"errors"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/sourcehealth"
	"gorm.io/gorm"
)

type sourceProbeArgs struct {
	Source string `json:"source"`
}

func (m *Manager) ConfigureSourceProbes(load func(context.Context) (ScheduleSettings, error), probe func(context.Context, string) error) {
	m.sourceProbeLoad = load
	m.sourceProbesReady = true
	m.queue.Register("source_probe", func(ctx context.Context, args sourceProbeArgs) error {
		return protectTaskError(func() error {
			settings, err := load(ctx)
			if err != nil {
				return err
			}
			schedule := settings.NewSourceSchedule(args.Source)
			if schedule == nil || !schedule.Auto {
				return nil
			}
			if err := probe(ctx, args.Source); err != nil {
				return jobs.NoRetry(err)
			}
			return nil
		})
	})
}

func (m *Manager) EnsureSourceProbes(ctx context.Context) error {
	settings, err := m.sourceProbeLoad(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, source := range newHistorySources {
		schedule := settings.NewSourceSchedule(source)
		if schedule == nil || !schedule.Auto {
			continue
		}
		var state sourcehealth.State
		err := m.db.WithContext(ctx).First(&state, "source = ?", source).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if state.Verified || state.NextRetry != nil && state.NextRetry.After(now) || state.LastChecked != nil && state.LastChecked.After(now.Add(-30*time.Minute)) {
			continue
		}
		_, err = m.queue.Enqueue(ctx, "source_probe", sourceProbeArgs{Source: source}, jobs.QueueOpt("source_backfill"), jobs.Unique("source_probe:"+source), jobs.Retries(0), jobs.Timeout(5*time.Minute))
		if err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
			return err
		}
	}
	return nil
}
