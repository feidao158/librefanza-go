package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"gorm.io/gorm/clause"
)

var ErrCrawlInProgress = errors.New("source crawl already in progress")

type CrawlProgress struct {
	JobID      string    `json:"job_id" gorm:"primaryKey"`
	Source     string    `json:"source"`
	Mode       string    `json:"mode"`
	NextPage   int       `json:"next_page"`
	NextNumber int       `json:"next_number"`
	Processed  int       `json:"processed"`
	Undated    int       `json:"undated"`
	Skipped    int       `json:"skipped"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (CrawlProgress) TableName() string { return "source_crawl_progress" }

func (m *Manager) ConfigureCrawls(run func(context.Context, sources.CrawlRequest, sources.CrawlProgress, func(sources.CrawlProgress) error) error) {
	m.queue.Register("source_crawl", func(ctx context.Context, request sources.CrawlRequest) error {
		return protectTaskError(func() error {
			if err := request.Validate(); err != nil {
				if errors.Is(err, sources.ErrSourceRetired) {
					return jobs.NoRetry(err)
				}
				return err
			}
			id := jobs.JobIDFromContext(ctx).String()
			row := CrawlProgress{JobID: id, Source: request.Source, Mode: request.Mode, UpdatedAt: time.Now().UTC()}
			if err := m.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			if err := m.db.WithContext(ctx).First(&row, "job_id = ?", id).Error; err != nil {
				return err
			}
			progress := sources.CrawlProgress{NextPage: row.NextPage, NextNumber: row.NextNumber, Processed: row.Processed, Undated: row.Undated, Skipped: row.Skipped}
			err := run(ctx, request, progress, func(value sources.CrawlProgress) error {
				return m.db.WithContext(ctx).Model(&CrawlProgress{}).Where("job_id = ?", id).Updates(map[string]any{"next_page": value.NextPage, "next_number": value.NextNumber, "processed": value.Processed, "undated": value.Undated, "skipped": value.Skipped, "updated_at": time.Now().UTC()}).Error
			})
			var deferred *sources.DeferredError
			if errors.As(err, &deferred) {
				delay := time.Until(deferred.RetryAt)
				if delay < time.Second {
					delay = time.Second
				}
				return jobs.RetryAfter(delay, err)
			}
			if job := jobs.JobFromContext(ctx); err != nil && job != nil && job.Attempt >= 3 {
				return jobs.NoRetry(err)
			}
			return err
		})
	})
}

func (m *Manager) EnqueueCrawl(ctx context.Context, request sources.CrawlRequest) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	key := "source_crawl:" + request.Source
	id, err := m.queue.Enqueue(ctx, "source_crawl", request, jobs.QueueOpt("crawl"), jobs.Unique(key), jobs.Retries(100), jobs.Timeout(30*24*time.Hour))
	if errors.Is(err, jobs.ErrDuplicateJob) {
		var existing jobs.Job
		if err := m.db.WithContext(ctx).Where("unique_key = ? AND status IN ?", key, []string{"pending", "running", "retrying", "waiting", "paused"}).Order("created_at DESC").First(&existing).Error; err != nil {
			return "", err
		}
		var previous sources.CrawlRequest
		if json.Unmarshal(existing.Args, &previous) != nil || !reflect.DeepEqual(request, previous) {
			return existing.ID.String(), ErrCrawlInProgress
		}
		return existing.ID.String(), nil
	}
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
