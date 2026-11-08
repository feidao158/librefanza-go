package tasks

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/sourcehealth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sourceHistoryProgress struct {
	Source    string `gorm:"primaryKey"`
	CursorID  uint64
	LimitID   uint64
	Processed int64
	Failed    int64
	Completed bool
	UpdatedAt time.Time
}

func (sourceHistoryProgress) TableName() string { return "source_history_progress" }

type coverRepairDeferred struct {
	SourceItemID uint64 `gorm:"primaryKey"`
	Attempts     int
	NextRetry    time.Time
	UpdatedAt    time.Time
}

func (coverRepairDeferred) TableName() string { return "cover_repair_deferred" }

var newHistorySources = []string{"javdb", "javbooks", "javquery"}

func (m *Manager) ConfigureSourceBackfill(collect func(context.Context, string, string) (bool, error), improve func(context.Context, catalog.SourceItem) error) {
	m.queue.Register("source_history_backfill", func(ctx context.Context, args sourceHistoryArgs) error {
		return protectTaskError(func() error {
			if args.Source == "covers" {
				return m.runCoverBackfill(ctx, improve)
			}
			return m.runSourceHistoryBackfill(ctx, args.Source, collect)
		})
	})
	m.sourceBackfillReady = true
}

type sourceHistoryArgs struct {
	Source string `json:"source"`
}

func (m *Manager) EnsureSourceBackfill(ctx context.Context) error {
	if err := migrateSourceBackfillJobs(ctx, m.db); err != nil {
		return err
	}
	now := time.Now().UTC()
	var coverLimit uint64
	if err := m.db.WithContext(ctx).Model(&catalog.SourceItem{}).Select("COALESCE(MAX(id), 0)").Scan(&coverLimit).Error; err != nil {
		return err
	}
	cover := sourceHistoryProgress{Source: "covers", LimitID: coverLimit, UpdatedAt: now}
	if err := m.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&cover).Error; err != nil {
		return err
	}
	var movieLimit uint64
	if err := m.db.WithContext(ctx).Model(&catalog.Movie{}).Select("COALESCE(MAX(id), 0)").Scan(&movieLimit).Error; err != nil {
		return err
	}
	for _, source := range newHistorySources {
		var existing sourceHistoryProgress
		err := m.db.WithContext(ctx).First(&existing, "source = ?", source).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			continue
		}
		var health sourcehealth.State
		if err := m.db.WithContext(ctx).Where("source = ? AND status = ? AND verified = ?", source, "available", true).First(&health).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return err
		}
		var count int64
		if err := m.db.WithContext(ctx).Model(&catalog.SourceItem{}).Where("source = ?", source).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			row := sourceHistoryProgress{Source: source, LimitID: movieLimit, UpdatedAt: now}
			if err := m.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
	}
	var progress []sourceHistoryProgress
	if err := m.db.WithContext(ctx).Where("completed = ?", false).Find(&progress).Error; err != nil {
		return err
	}
	var states []sourcehealth.State
	if err := m.db.WithContext(ctx).Find(&states).Error; err != nil {
		return err
	}
	bySource := make(map[string]sourcehealth.State, len(states))
	for _, state := range states {
		bySource[state.Source] = state
	}
	for _, row := range progress {
		if row.Source == "javbus" {
			continue
		}
		state := bySource[row.Source]
		if state.Status == "cooldown" && state.NextRetry != nil && state.NextRetry.After(now) {
			continue
		}
		if err := m.enqueueSourceHistory(ctx, row.Source); err != nil {
			return err
		}
	}
	var due int64
	if err := m.db.WithContext(ctx).Model(&coverRepairDeferred{}).Where("next_retry <= ?", now).Count(&due).Error; err != nil {
		return err
	}
	if due > 0 {
		return m.enqueueSourceHistory(ctx, "covers")
	}
	return nil
}

func (m *Manager) enqueueSourceHistory(ctx context.Context, source string) error {
	_, err := m.queue.Enqueue(ctx, "source_history_backfill", sourceHistoryArgs{Source: source}, jobs.QueueOpt("source_backfill"), jobs.Unique("source_history_backfill:"+source), jobs.Retries(3), jobs.Timeout(24*time.Hour))
	if errors.Is(err, jobs.ErrDuplicateJob) {
		return nil
	}
	return err
}

func (m *Manager) runSourceHistoryBackfill(ctx context.Context, source string, collect func(context.Context, string, string) (bool, error)) error {
	valid := false
	for _, candidate := range newHistorySources {
		valid = valid || candidate == source
	}
	if !valid {
		return jobs.NoRetry(errors.New("invalid source"))
	}
	var progress sourceHistoryProgress
	if err := m.db.WithContext(ctx).First(&progress, "source = ?", source).Error; err != nil {
		return err
	}
	if progress.Completed {
		return nil
	}
	var health sourcehealth.State
	if err := m.db.WithContext(ctx).Where("source = ?", source).First(&health).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if health.Status == "cooldown" && health.NextRetry != nil && health.NextRetry.After(time.Now().UTC()) {
		return nil
	}
	var movies []catalog.Movie
	if err := m.db.WithContext(ctx).Select("id", "normalized_id", "is_hidden").Where("id > ? AND id <= ?", progress.CursorID, progress.LimitID).Order("id").Limit(25).Find(&movies).Error; err != nil {
		return err
	}
	if len(movies) == 0 {
		return m.db.WithContext(ctx).Model(&sourceHistoryProgress{}).Where("source = ?", source).Updates(map[string]any{"completed": true, "updated_at": time.Now().UTC()}).Error
	}
	for _, movie := range movies {
		if err := ctx.Err(); err != nil {
			return err
		}
		failed := false
		if !movie.IsHidden {
			var count int64
			if err := m.db.WithContext(ctx).Model(&catalog.SourceItem{}).Where("source = ? AND normalized_id = ?", source, movie.NormalizedID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				jobID := fmt.Sprintf("history:%s:%d", source, movie.ID)
				runErr := m.runEnrichment(ctx, jobID, movie.NormalizedID, []string{source}, collect)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				var state MetadataSourceProgress
				if err := m.db.WithContext(ctx).Where("job_id = ? AND source = ?", jobID, source).First(&state).Error; err != nil {
					if runErr != nil {
						return runErr
					}
					return err
				}
				failed = state.Status == "deferred"
				if runErr != nil && !failed {
					return runErr
				}
			}
		}
		changes := map[string]any{"cursor_id": movie.ID, "processed": gorm.Expr("processed + 1"), "updated_at": time.Now().UTC()}
		if failed {
			changes["failed"] = gorm.Expr("failed + 1")
		}
		if err := m.db.WithContext(ctx).Model(&sourceHistoryProgress{}).Where("source = ? AND cursor_id < ?", source, movie.ID).Updates(changes).Error; err != nil {
			return err
		}
		if failed {
			return nil
		}
	}
	return nil
}

var smallImagePath = regexp.MustCompile(`(?i)(?:^|/)(?:w|width)([0-9]{1,4})(?:h[0-9]+)?(?:/|$)`)

func suspiciousCover(item catalog.SourceItem) bool {
	if item.CoverImageURL == "" || item.CoverImageURL == item.ThumbnailImageURL {
		return true
	}
	address, err := url.Parse(item.CoverImageURL)
	if err != nil {
		return false
	}
	if strings.HasSuffix(strings.ToLower(address.Path), "ps.jpg") {
		return true
	}
	match := smallImagePath.FindStringSubmatch(address.Path)
	if len(match) > 1 {
		width, _ := strconv.Atoi(match[1])
		return width > 0 && width < 400
	}
	return false
}

func (m *Manager) runCoverBackfill(ctx context.Context, improve func(context.Context, catalog.SourceItem) error) error {
	var retries []coverRepairDeferred
	if err := m.db.WithContext(ctx).Where("next_retry <= ?", time.Now().UTC()).Order("next_retry").Limit(10).Find(&retries).Error; err != nil {
		return err
	}
	for _, retry := range retries {
		if err := m.repairCover(ctx, retry.SourceItemID, improve); err != nil {
			return err
		}
	}
	var progress sourceHistoryProgress
	if err := m.db.WithContext(ctx).First(&progress, "source = ?", "covers").Error; err != nil {
		return err
	}
	if progress.Completed {
		return nil
	}
	var items []catalog.SourceItem
	if err := m.db.WithContext(ctx).Where("id > ? AND id <= ?", progress.CursorID, progress.LimitID).Order("id").Limit(25).Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return m.db.WithContext(ctx).Model(&sourceHistoryProgress{}).Where("source = ?", "covers").Updates(map[string]any{"completed": true, "updated_at": time.Now().UTC()}).Error
	}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.Source != "avmoo" && item.Source != "javlibrary" && item.Source != "javbus" && suspiciousCover(item) {
			if err := m.repairCover(ctx, item.ID, improve); err != nil {
				return err
			}
		}
		if err := m.db.WithContext(ctx).Model(&sourceHistoryProgress{}).Where("source = ? AND cursor_id < ?", "covers", item.ID).Updates(map[string]any{"cursor_id": item.ID, "processed": gorm.Expr("processed + 1"), "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) repairCover(ctx context.Context, id uint64, improve func(context.Context, catalog.SourceItem) error) error {
	var item catalog.SourceItem
	if err := m.db.WithContext(ctx).First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return m.db.WithContext(ctx).Delete(&coverRepairDeferred{}, "source_item_id = ?", id).Error
		}
		return err
	}
	if item.Source == "javbus" {
		return m.db.WithContext(ctx).Delete(&coverRepairDeferred{}, "source_item_id = ?", id).Error
	}
	err := improve(ctx, item)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return m.db.WithContext(ctx).Delete(&coverRepairDeferred{}, "source_item_id = ?", id).Error
	}
	var old coverRepairDeferred
	if dbErr := m.db.WithContext(ctx).First(&old, "source_item_id = ?", id).Error; dbErr != nil && !errors.Is(dbErr, gorm.ErrRecordNotFound) {
		return dbErr
	}
	attempts := old.Attempts + 1
	shift := attempts - 1
	if shift > 7 {
		shift = 7
	}
	now := time.Now().UTC()
	row := coverRepairDeferred{SourceItemID: id, Attempts: attempts, NextRetry: now.Add(10 * time.Minute * time.Duration(1<<shift)), UpdatedAt: now}
	return m.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "source_item_id"}}, DoUpdates: clause.AssignmentColumns([]string{"attempts", "next_retry", "updated_at"})}).Create(&row).Error
}
