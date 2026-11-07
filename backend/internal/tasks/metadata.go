package tasks

import (
	"context"
	"errors"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/sourcehealth"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var metadataSources = []string{"fanza", "sod", "mgstage", "fc2", "jable", "libredmm", "javdb", "javbooks", "javquery"}
var ErrStoredSourceInvalid = errors.New("stored source item is invalid")

type MetadataSourceProgress struct {
	JobID      string    `gorm:"primaryKey" json:"job_id"`
	MovieID    string    `json:"movie_id"`
	Source     string    `gorm:"primaryKey" json:"source"`
	Status     string    `json:"status"`
	Attempts   int       `json:"attempts"`
	Reason     string    `json:"reason"`
	HTTPStatus int       `json:"http_status"`
	Found      bool      `json:"found"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (MetadataSourceProgress) TableName() string { return "metadata_source_progress" }

type MetadataBackfillProgress struct {
	ID            int       `gorm:"primaryKey" json:"-"`
	Phase         string    `json:"phase"`
	CursorID      uint64    `json:"cursor_id"`
	MovieCursorID uint64    `json:"movie_cursor_id"`
	RawLimitID    uint64    `json:"raw_limit_id"`
	MovieLimitID  uint64    `json:"movie_limit_id"`
	Processed     int64     `json:"processed"`
	Failed        int64     `json:"failed"`
	Completed     bool      `json:"completed"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (MetadataBackfillProgress) TableName() string { return "metadata_backfill_progress" }

type MetadataStatus struct {
	ID               string `json:"id"`
	Status           string `json:"status"`
	CompletedSources int64  `json:"completed_sources"`
	FailedSources    int64  `json:"failed_sources"`
	DeferredSources  int64  `json:"deferred_sources"`
	TotalSources     int    `json:"total_sources"`
	CollectionStatus string `json:"collection_status"`
}

func (m *Manager) EnrichmentStatus(ctx context.Context, id string) (MetadataStatus, error) {
	if !catalog.IsProductID(id) {
		return MetadataStatus{}, ErrJobNotFound
	}
	var visible catalog.Movie
	if err := m.db.WithContext(ctx).Where("normalized_id = ? AND is_hidden = ?", catalog.NormalizeProductID(id), false).First(&visible).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MetadataStatus{}, ErrJobNotFound
		}
		return MetadataStatus{}, err
	}
	var job jobs.Job
	err := m.db.WithContext(ctx).Where("type = ? AND unique_key = ?", "metadata_enrich", "metadata_enrich:"+catalog.NormalizeProductID(id)).Order("created_at DESC").First(&job).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MetadataStatus{}, ErrJobNotFound
		}
		return MetadataStatus{}, err
	}
	status := MetadataStatus{ID: job.ID.String(), Status: string(job.Status), TotalSources: len(metadataSources)}
	var current []MetadataSourceProgress
	if err := m.db.WithContext(ctx).Where("job_id = ? AND source IN ?", status.ID, metadataSources).Find(&current).Error; err != nil {
		return MetadataStatus{}, err
	}
	var historical []MetadataSourceProgress
	if err := m.db.WithContext(ctx).Where("movie_id = ? AND source IN ? AND job_id LIKE ?", catalog.NormalizeProductID(id), newHistorySources, "history:%").Order("updated_at DESC").Find(&historical).Error; err != nil {
		return MetadataStatus{}, err
	}
	states := make(map[string]string, len(metadataSources))
	for _, row := range current {
		states[row.Source] = row.Status
	}
	for _, row := range historical {
		if states[row.Source] == "" {
			states[row.Source] = row.Status
		}
	}
	for _, state := range states {
		switch state {
		case "completed":
			status.CompletedSources++
		case "failed":
			status.FailedSources++
		case "deferred":
			status.DeferredSources++
		}
	}
	status.CollectionStatus = "collecting"
	if status.CompletedSources == int64(status.TotalSources) {
		status.CollectionStatus = "complete"
	} else if status.DeferredSources > 0 {
		status.CollectionStatus = "deferred"
	} else if status.Status == "failed" || status.Status == "cancelled" {
		status.CollectionStatus = "failed"
	}
	return status, nil
}

func (m *Manager) runEnrichment(ctx context.Context, jobID, id string, names []string, collect func(context.Context, string, string) (bool, error)) error {
	var failures []error
	for _, name := range names {
		var progress MetadataSourceProgress
		if id != "" {
			var recent MetadataSourceProgress
			err := m.db.WithContext(ctx).Where("job_id <> ? AND movie_id = ? AND source = ? AND status = ? AND found = ? AND updated_at > ?", jobID, id, name, "completed", true, time.Now().UTC().Add(-24*time.Hour)).Order("updated_at DESC").First(&recent).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil {
				var count int64
				if err := m.db.WithContext(ctx).Model(&catalog.SourceItem{}).Where("normalized_id = ? AND source = ?", id, name).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					progress = MetadataSourceProgress{JobID: jobID, MovieID: id, Source: name, Status: "completed", Found: false, UpdatedAt: recent.UpdatedAt}
					if err := m.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "job_id"}, {Name: "source"}}, DoUpdates: clause.AssignmentColumns([]string{"movie_id", "status", "found", "updated_at"})}).Create(&progress).Error; err != nil {
						return err
					}
					continue
				}
			}
		}
		err := m.db.WithContext(ctx).Where("job_id = ? AND source = ?", jobID, name).First(&progress).Error
		if err == nil && progress.Status == "completed" {
			continue
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		found, collectErr := collect(ctx, id, name)
		status := "completed"
		entry := logbook.Entry{}
		if collectErr != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			status = "deferred"
			logbook.ApplyFailure(&entry, collectErr)
			var deferred *sources.DeferredError
			if !errors.As(collectErr, &deferred) {
				failures = append(failures, collectErr)
			}
			m.RecordSourceFailure(ctx, name, collectErr)
		}
		attempts := progress.Attempts + 1
		progress = MetadataSourceProgress{JobID: jobID, MovieID: id, Source: name, Status: status, Attempts: attempts, Reason: entry.Reason, HTTPStatus: entry.HTTPStatus, Found: found, UpdatedAt: time.Now().UTC()}
		if err := m.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "job_id"}, {Name: "source"}}, DoUpdates: clause.AssignmentColumns([]string{"movie_id", "status", "attempts", "reason", "http_status", "found", "updated_at"})}).Create(&progress).Error; err != nil {
			return err
		}
		var saveErr *sources.SaveError
		if errors.As(collectErr, &saveErr) {
			return collectErr
		}
	}
	return errors.Join(failures...)
}

func (m *Manager) ConfigureMetadata(collect func(context.Context, string, string) (bool, error), reparse func(context.Context, catalog.SourceItem) error) error {
	m.enrichmentReady = true
	m.queue.Register("metadata_enrich", func(ctx context.Context, args SearchArgs) error {
		return protectTaskError(func() error {
			return m.runEnrichment(ctx, jobs.JobIDFromContext(ctx).String(), args.ID, metadataSources, collect)
		})
	})
	m.queue.Register("metadata_backfill", func(ctx context.Context, _ struct{}) error {
		return protectTaskError(func() error { return m.runBackfill(ctx, reparse) })
	})
	m.queue.Register("metadata_backfill_tick", func(ctx context.Context, _ struct{}) error {
		return protectTaskError(func() error {
			if err := m.EnqueueDeferredMetadata(ctx); err != nil {
				return err
			}
			if m.sourceBackfillReady {
				if err := m.EnsureSourceBackfill(ctx); err != nil {
					return err
				}
			}
			if m.sourceProbesReady {
				if err := m.EnsureSourceProbes(ctx); err != nil {
					return err
				}
			}
			return m.EnsureBackfill(ctx)
		})
	})
	m.queue.Register("metadata_source_retry", func(ctx context.Context, args metadataRetryArgs) error {
		return protectTaskError(func() error { return m.runEnrichment(ctx, args.JobID, args.MovieID, []string{args.Source}, collect) })
	})
	schedule, err := jobs.Cron("* * * * *")
	if err != nil {
		return err
	}
	return m.queue.Schedule("metadata_backfill_tick", struct{}{}, schedule, jobs.Unique("metadata_backfill_tick"))
}

type metadataRetryArgs struct {
	JobID   string `json:"job_id"`
	MovieID string `json:"movie_id"`
	Source  string `json:"source"`
}

func (m *Manager) EnqueueDeferredMetadata(ctx context.Context) error {
	var states []sourcehealth.State
	if err := m.db.WithContext(ctx).Find(&states).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	bySource := make(map[string]sourcehealth.State, len(states))
	for _, state := range states {
		bySource[state.Source] = state
	}
	for _, source := range metadataSources {
		state := bySource[source]
		if state.NextRetry != nil && state.NextRetry.After(now) {
			continue
		}
		limit := 10
		if state.Status == "cooldown" {
			limit = 1
		}
		var rows []MetadataSourceProgress
		if err := m.db.WithContext(ctx).Where("status = ? AND source = ? AND updated_at < ?", "deferred", source, now.Add(-time.Minute)).Order("updated_at").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			args := metadataRetryArgs{JobID: row.JobID, MovieID: row.MovieID, Source: row.Source}
			_, err := m.queue.Enqueue(ctx, "metadata_source_retry", args, jobs.QueueOpt("metadata"), jobs.Unique("metadata_retry:"+row.JobID+":"+row.Source), jobs.Retries(3), jobs.Timeout(24*time.Hour))
			if err != nil && !errors.Is(err, jobs.ErrDuplicateJob) {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) EnqueueEnrichment(ctx context.Context, id string) (string, error) {
	if !catalog.IsProductID(id) {
		return "", ErrInvalidID
	}
	id = catalog.NormalizeProductID(id)
	key := "metadata_enrich:" + id
	jobID, err := m.queue.Enqueue(ctx, "metadata_enrich", SearchArgs{ID: id}, jobs.QueueOpt("metadata"), jobs.Unique(key), jobs.Retries(3), jobs.Timeout(24*time.Hour))
	if errors.Is(err, jobs.ErrDuplicateJob) {
		var existing jobs.Job
		if err := m.db.WithContext(ctx).Where("unique_key = ? AND status IN ?", key, []string{"pending", "running", "retrying", "waiting", "paused"}).Order("created_at DESC").First(&existing).Error; err != nil {
			return "", err
		}
		return existing.ID.String(), nil
	}
	if err != nil {
		return "", err
	}
	return jobID.String(), nil
}

func (m *Manager) EnsureBackfill(ctx context.Context) error {
	var progress MetadataBackfillProgress
	if err := m.db.WithContext(ctx).First(&progress, 1).Error; err != nil {
		return err
	}
	var active int64
	if err := m.db.WithContext(ctx).Model(&jobs.Job{}).Where("type = ? AND status IN ?", "metadata_backfill", []string{"pending", "running", "retrying", "waiting", "paused"}).Count(&active).Error; err != nil {
		return err
	}
	if active > 0 {
		return nil
	}
	if progress.Completed {
		var sourceID, movieID uint64
		if err := m.db.WithContext(ctx).Model(&catalog.SourceItem{}).Select("COALESCE(MAX(id), 0)").Scan(&sourceID).Error; err != nil {
			return err
		}
		if err := m.db.WithContext(ctx).Model(&catalog.Movie{}).Select("COALESCE(MAX(id), 0)").Scan(&movieID).Error; err != nil {
			return err
		}
		if sourceID <= progress.RawLimitID && movieID <= progress.MovieLimitID {
			return nil
		}
		result := m.db.WithContext(ctx).Model(&MetadataBackfillProgress{}).Where("id = ? AND completed = ? AND raw_limit_id = ? AND movie_limit_id = ?", 1, true, progress.RawLimitID, progress.MovieLimitID).Updates(map[string]any{"phase": "raw", "cursor_id": progress.RawLimitID, "raw_limit_id": sourceID, "movie_limit_id": movieID, "completed": false})
		if result.Error != nil {
			return result.Error
		}
	}
	var failed int64
	if err := m.db.WithContext(ctx).Model(&jobs.Job{}).Where("type = ? AND status IN ?", "metadata_backfill", []string{"failed", "cancelled", "paused"}).Count(&failed).Error; err != nil {
		return err
	}
	if failed > 0 {
		return nil
	}
	_, err := m.queue.Enqueue(ctx, "metadata_backfill", struct{}{}, jobs.QueueOpt("backfill"), jobs.Unique("metadata_backfill"), jobs.Retries(3), jobs.Timeout(30*24*time.Hour))
	if errors.Is(err, jobs.ErrDuplicateJob) {
		return nil
	}
	return err
}

func (m *Manager) runBackfill(ctx context.Context, reparse func(context.Context, catalog.SourceItem) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var progress MetadataBackfillProgress
		if err := m.db.WithContext(ctx).First(&progress, 1).Error; err != nil {
			return err
		}
		if progress.Completed {
			return nil
		}
		if progress.Phase == "raw" {
			var items []catalog.SourceItem
			if err := m.db.WithContext(ctx).Where("id > ? AND id <= ?", progress.CursorID, progress.RawLimitID).Order("id").Limit(100).Find(&items).Error; err != nil {
				return err
			}
			if len(items) == 0 {
				if err := m.db.WithContext(ctx).Model(&progress).Updates(map[string]any{"phase": "movies", "cursor_id": progress.MovieCursorID}).Error; err != nil {
					return err
				}
				continue
			}
			for _, item := range items {
				if err := ctx.Err(); err != nil {
					return err
				}
				changes := map[string]any{"cursor_id": item.ID, "processed": gorm.Expr("processed + ?", 1)}
				if err := reparse(ctx, item); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if !errors.Is(err, ErrStoredSourceInvalid) {
						return err
					}
					changes["failed"] = gorm.Expr("failed + ?", 1)
				}
				if err := m.db.WithContext(ctx).Model(&progress).Updates(changes).Error; err != nil {
					return err
				}
			}
			continue
		}
		var movies []catalog.Movie
		if err := m.db.WithContext(ctx).Where("id > ? AND id <= ?", progress.CursorID, progress.MovieLimitID).Order("id").Limit(100).Find(&movies).Error; err != nil {
			return err
		}
		if len(movies) == 0 {
			return m.db.WithContext(ctx).Model(&progress).Update("completed", true).Error
		}
		for _, movie := range movies {
			var previous int64
			if err := m.db.WithContext(ctx).Model(&jobs.Job{}).Where("type = ? AND unique_key = ?", "metadata_enrich", "metadata_enrich:"+movie.NormalizedID).Count(&previous).Error; err != nil {
				return err
			}
			if previous > 0 {
				if err := m.db.WithContext(ctx).Model(&progress).Updates(map[string]any{"cursor_id": movie.ID, "movie_cursor_id": movie.ID, "processed": gorm.Expr("processed + ?", 1)}).Error; err != nil {
					return err
				}
				continue
			}
			for {
				var count int64
				if err := m.db.WithContext(ctx).Model(&jobs.Job{}).Where("type = ? AND status IN ?", "metadata_enrich", []string{"pending", "running", "retrying", "waiting"}).Count(&count).Error; err != nil {
					return err
				}
				if count < 100 {
					break
				}
				timer := time.NewTimer(time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			if _, err := m.EnqueueEnrichment(ctx, movie.NormalizedID); err != nil {
				return err
			}
			if err := m.db.WithContext(ctx).Model(&progress).Updates(map[string]any{"cursor_id": movie.ID, "movie_cursor_id": movie.ID, "processed": gorm.Expr("processed + ?", 1)}).Error; err != nil {
				return err
			}
		}
	}
}
