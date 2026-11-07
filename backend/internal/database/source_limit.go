package database

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sourceRequestLimit struct {
	Source string `gorm:"primaryKey"`
	NextAt time.Time
}

func (sourceRequestLimit) TableName() string { return "source_request_limits" }

type SourceRateLimiter struct {
	db       *gorm.DB
	interval time.Duration
}

func NewSourceRateLimiter(db *gorm.DB) *SourceRateLimiter {
	return newSourceRateLimiter(db, time.Second)
}

func newSourceRateLimiter(db *gorm.DB, interval time.Duration) *SourceRateLimiter {
	return &SourceRateLimiter{db: db, interval: interval}
}

func (l *SourceRateLimiter) Wait(ctx context.Context, source string) error {
	for {
		var delay time.Duration
		err := l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			now, err := databaseTime(tx)
			if err != nil {
				return err
			}
			row := sourceRequestLimit{Source: source, NextAt: now}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "source = ?", source).Error; err != nil {
				return err
			}
			if row.NextAt.After(now) {
				delay = row.NextAt.Sub(now)
				return nil
			}
			return tx.Model(&row).Update("next_at", now.Add(l.interval)).Error
		})
		if err != nil {
			return err
		}
		if delay <= 0 {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *SourceRateLimiter) Pause(ctx context.Context, source string, until time.Time) error {
	delay := time.Until(until)
	if delay < 0 {
		delay = 0
	}
	return l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now, err := databaseTime(tx)
		if err != nil {
			return err
		}
		deadline := now.Add(delay)
		row := sourceRequestLimit{Source: source, NextAt: deadline}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "source = ?", source).Error; err != nil {
			return err
		}
		if !deadline.After(row.NextAt) {
			return nil
		}
		return tx.Model(&row).Update("next_at", deadline).Error
	})
}

func databaseTime(tx *gorm.DB) (time.Time, error) {
	var result struct{ Now time.Time }
	err := tx.Raw("SELECT CURRENT_TIMESTAMP(6) AS now").Scan(&result).Error
	return result.Now.UTC(), err
}
