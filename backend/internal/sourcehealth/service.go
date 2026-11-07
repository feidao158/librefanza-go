package sourcehealth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type State struct {
	Source      string     `json:"source" gorm:"primaryKey"`
	Status      string     `json:"status"`
	Verified    bool       `json:"verified"`
	Failures    int        `json:"failures"`
	Reason      string     `json:"reason"`
	HTTPStatus  int        `json:"http_status"`
	LastChecked *time.Time `json:"last_checked"`
	NextRetry   *time.Time `json:"next_retry"`
	ReportedAt  *time.Time `json:"-"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (State) TableName() string { return "source_health" }

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Begin(ctx context.Context, source string) (time.Time, error) {
	var deferred time.Time
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		row := State{Source: source, Status: "unknown", UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "source = ?", source).Error; err != nil {
			return err
		}
		if row.NextRetry != nil && row.NextRetry.After(now) {
			deferred = *row.NextRetry
			return nil
		}
		if row.Status == "cooldown" {
			lease := now.Add(time.Minute)
			return tx.Model(&row).Updates(map[string]any{"next_retry": lease, "updated_at": now}).Error
		}
		return nil
	})
	return deferred, err
}

func (s *Service) End(ctx context.Context, source string, cause error) (bool, error) {
	report := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row State
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "source = ?", source).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		changes := map[string]any{"last_checked": now, "updated_at": now}
		if cause == nil {
			changes["status"], changes["failures"], changes["reason"], changes["http_status"], changes["next_retry"] = "available", 0, "", 0, nil
		} else {
			entry := logbook.Entry{}
			logbook.ApplyFailure(&entry, cause)
			failures := row.Failures + 1
			delay := cooldown(cause, failures)
			status := "degraded"
			changes["next_retry"] = nil
			if delay > 0 {
				status = "cooldown"
				changes["next_retry"] = now.Add(delay)
			}
			report = row.Reason != entry.Reason || row.HTTPStatus != entry.HTTPStatus || row.Status != status || row.ReportedAt == nil || now.Sub(*row.ReportedAt) >= time.Hour
			if report {
				changes["reported_at"] = now
			}
			changes["status"], changes["failures"], changes["reason"], changes["http_status"] = status, failures, entry.Reason, entry.HTTPStatus
		}
		return tx.Model(&row).Updates(changes).Error
	})
	return report, err
}

func (s *Service) List(ctx context.Context) ([]State, error) {
	var rows []State
	err := s.db.WithContext(ctx).Order("source").Find(&rows).Error
	return rows, err
}

func (s *Service) MarkVerified(ctx context.Context, source string) error {
	return s.db.WithContext(ctx).Model(&State{}).Where("source = ?", source).Updates(map[string]any{"verified": true, "updated_at": time.Now().UTC()}).Error
}

func cooldown(err error, failures int) time.Duration {
	var request *sources.RequestError
	if strings.Contains(err.Error(), "not configured") {
		return 30 * time.Minute
	}
	if errors.As(err, &request) && request.Status == 429 {
		if delay := time.Until(request.RetryAt); delay > 0 {
			return delay
		}
		return time.Minute
	}
	if errors.As(err, &request) && (request.Status == 401 || request.Status == 403) || strings.Contains(err.Error(), "source requires verification") {
		shift := failures - 1
		if shift < 0 {
			shift = 0
		}
		if shift > 4 {
			shift = 4
		}
		return 30 * time.Minute * time.Duration(1<<shift)
	}
	if failures < 3 {
		return 0
	}
	shift := failures - 3
	if shift > 6 {
		shift = 6
	}
	return 5 * time.Minute * time.Duration(1<<shift)
}
