package recommendation

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	gorse "github.com/gorse-io/gorse-go"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type managedSyncClient interface {
	InsertItem(context.Context, gorse.Item) (gorse.RowAffected, error)
	DeleteFeedbacks(context.Context, string, string) (gorse.RowAffected, error)
	InsertFeedback(context.Context, []gorse.Feedback) (gorse.RowAffected, error)
}

type managedSyncState struct {
	Name      string `gorm:"primaryKey"`
	Value     string
	UpdatedAt time.Time
}

func (managedSyncState) TableName() string { return "settings" }

type managedSyncProgress struct {
	Phase     string    `json:"phase"`
	Cursor    uint64    `json:"cursor"`
	Ready     bool      `json:"ready"`
	NextCycle time.Time `json:"next_cycle"`
	ChangedAt time.Time `json:"changed_at,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
}

type managedSync struct {
	db     *gorm.DB
	client managedSyncClient
	name   string
}

const managedSyncName = "gorse_managed_sync"
const managedSyncHealthName = "gorse_sync_health"

var ErrManagedSyncIdle = errors.New("recommendation synchronization idle")

// SetManagedSyncHealthy 保存不含异常正文的同步健康状态。
func SetManagedSyncHealthy(ctx context.Context, db *gorm.DB, healthy bool) error {
	data, err := json.Marshal(struct {
		Healthy bool `json:"healthy"`
	}{Healthy: healthy})
	if err != nil {
		return err
	}
	state := managedSyncState{Name: managedSyncHealthName, Value: string(data), UpdatedAt: time.Now().UTC()}
	return db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&state).Error
}

// ManagedSyncHealthy 读取最近一次同步是否失败。
func ManagedSyncHealthy(ctx context.Context, db *gorm.DB) (bool, error) {
	var state managedSyncState
	err := db.WithContext(ctx).Where("name = ?", managedSyncHealthName).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	var health struct {
		Healthy bool `json:"healthy"`
	}
	if err := json.Unmarshal([]byte(state.Value), &health); err != nil {
		return false, err
	}
	return health.Healthy, nil
}

// ManagedSyncStatus 返回内置推荐服务最近一次对账进度。
func ManagedSyncStatus(ctx context.Context, db *gorm.DB) (managedSyncProgress, error) {
	var state managedSyncState
	if err := db.WithContext(ctx).Where("name = ?", managedSyncName).First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return managedSyncProgress{}, nil
		}
		return managedSyncProgress{}, err
	}
	var progress managedSyncProgress
	err := json.Unmarshal([]byte(state.Value), &progress)
	return progress, err
}

// ManagedSyncStep 执行一条可重试的内置推荐对账。
func ManagedSyncStep(ctx context.Context, db *gorm.DB, cfg settings.GorseSettings) error {
	if !cfg.Enabled || cfg.EntryPoint == "" {
		return ErrManagedSyncIdle
	}
	client := NewGorseClient(cfg.EntryPoint, cfg.APIKey)
	progress, err := ManagedSyncStatus(ctx, db)
	if err != nil {
		return err
	}
	if progress.Ready {
		err = newManagedSync(db, client, "gorse_recent_feedback").recentStep(ctx)
		if !errors.Is(err, ErrManagedSyncIdle) {
			return err
		}
		err = newManagedSync(db, client, "gorse_recent_movies").recentMovieStep(ctx, progress.StartedAt)
		if !errors.Is(err, ErrManagedSyncIdle) {
			return err
		}
	}
	return newManagedSync(db, client, managedSyncName).step(ctx)
}

// recentMovieStep 增量同步影片变更，周期性全量对账修复边界情况。
func (s *managedSync) recentMovieStep(ctx context.Context, since time.Time) error {
	initialData, err := json.Marshal(managedSyncProgress{ChangedAt: since})
	if err != nil {
		return err
	}
	initial := managedSyncState{Name: s.name, Value: string(initialData), UpdatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state managedSyncState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("name = ?", s.name).First(&state).Error; err != nil {
			return err
		}
		var progress managedSyncProgress
		if err := json.Unmarshal([]byte(state.Value), &progress); err != nil {
			return err
		}
		var movie catalog.Movie
		query := tx.Order("updated_at ASC, id ASC")
		if !progress.ChangedAt.IsZero() {
			query = query.Where("updated_at > ? OR (updated_at = ? AND id > ?)", progress.ChangedAt, progress.ChangedAt, progress.Cursor)
		}
		err := query.First(&movie).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedSyncIdle
		}
		if err != nil {
			return err
		}
		if err := s.syncMovie(ctx, tx, movie); err != nil {
			return err
		}
		progress.ChangedAt, progress.Cursor = movie.UpdatedAt, movie.ID
		data, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		return tx.Model(&state).Updates(map[string]any{"value": string(data), "updated_at": time.Now().UTC()}).Error
	})
}

func newManagedSync(db *gorm.DB, client managedSyncClient, name string) *managedSync {
	return &managedSync{db: db, client: client, name: name}
}

func managedUserID(id string) string {
	mac := hmac.New(sha256.New, []byte(os.Getenv("GORSE_API_KEY")))
	_, _ = mac.Write([]byte(id))
	return "u_" + hex.EncodeToString(mac.Sum(nil))
}

func (s *managedSync) syncMovie(ctx context.Context, tx *gorm.DB, movie catalog.Movie) error {
	features, err := (&Service{db: tx}).extractFeatures(ctx, movie)
	if err != nil {
		return err
	}
	var labels []string
	for _, value := range features.Actresses {
		labels = append(labels, "actress:"+value)
	}
	for _, value := range features.Genres {
		labels = append(labels, "genre:"+value)
	}
	for _, value := range features.Makers {
		labels = append(labels, "maker:"+value)
	}
	date := movie.CreatedAt
	if movie.ReleaseDate != nil {
		date = *movie.ReleaseDate
	}
	item := gorse.Item{ItemId: movie.NormalizedID, IsHidden: movie.IsHidden, Categories: features.Genres, Labels: labels, Timestamp: date, Comment: features.Title}
	_, err = s.client.InsertItem(ctx, item)
	return err
}

func (s *managedSync) syncFeedback(ctx context.Context, tx *gorm.DB, feedback UserFeedback) error {
	var movie catalog.Movie
	err := tx.Where("normalized_id = ?", feedback.NormalizedID).First(&movie).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.syncMovie(ctx, tx, movie); err != nil {
		return err
	}
	userID := managedUserID(feedback.UserID)
	if _, err := s.client.DeleteFeedbacks(ctx, userID, feedback.NormalizedID); err != nil {
		return err
	}
	kind := feedback.FeedbackType
	if kind == "uncertain" {
		kind = "read"
	}
	entry := gorse.Feedback{FeedbackType: kind, UserId: userID, ItemId: feedback.NormalizedID, Value: feedback.Score, Timestamp: feedback.UpdatedAt}
	_, err = s.client.InsertFeedback(ctx, []gorse.Feedback{entry})
	return err
}

// recentStep 优先同步更新过的反馈，定期全量对账仍负责修复时间边界。
func (s *managedSync) recentStep(ctx context.Context) error {
	initial := managedSyncState{Name: s.name, Value: `{}`, UpdatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state managedSyncState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("name = ?", s.name).First(&state).Error; err != nil {
			return err
		}
		var progress managedSyncProgress
		if err := json.Unmarshal([]byte(state.Value), &progress); err != nil {
			return err
		}
		var feedback UserFeedback
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("updated_at ASC, id ASC")
		if !progress.ChangedAt.IsZero() {
			query = query.Where("updated_at > ? OR (updated_at = ? AND id > ?)", progress.ChangedAt, progress.ChangedAt, progress.Cursor)
		}
		err := query.First(&feedback).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrManagedSyncIdle
		}
		if err != nil {
			return err
		}
		if err := s.syncFeedback(ctx, tx, feedback); err != nil {
			return err
		}
		progress.ChangedAt, progress.Cursor = feedback.UpdatedAt, feedback.ID
		data, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		return tx.Model(&state).Updates(map[string]any{"value": string(data), "updated_at": time.Now().UTC()}).Error
	})
}

// step 将游标和单条同步结果一起提交，失败时下次从原位置重试。
func (s *managedSync) step(ctx context.Context) error {
	initialData, err := json.Marshal(managedSyncProgress{Phase: "movies", StartedAt: time.Now().UTC().Add(-time.Minute)})
	if err != nil {
		return err
	}
	initial := managedSyncState{Name: s.name, Value: string(initialData), UpdatedAt: time.Now().UTC()}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&initial).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state managedSyncState
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("name = ?", s.name).First(&state).Error; err != nil {
			return err
		}
		var progress managedSyncProgress
		if err := json.Unmarshal([]byte(state.Value), &progress); err != nil {
			return err
		}
		if progress.Phase == "" {
			progress.Phase = "movies"
		}
		if progress.Ready && time.Now().Before(progress.NextCycle) {
			return ErrManagedSyncIdle
		}
		switch progress.Phase {
		case "movies":
			var movie catalog.Movie
			err := tx.Where("id > ?", progress.Cursor).Order("id ASC").First(&movie).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				progress.Phase, progress.Cursor = "feedback", 0
			} else if err != nil {
				return err
			} else {
				if err := s.syncMovie(ctx, tx, movie); err != nil {
					return err
				}
				progress.Cursor = movie.ID
			}
		case "feedback":
			var feedback UserFeedback
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id > ?", progress.Cursor).Order("id ASC").First(&feedback).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				progress.Phase, progress.Cursor, progress.Ready = "movies", 0, true
				progress.NextCycle = time.Now().UTC().Add(12 * time.Hour)
			} else if err != nil {
				return err
			} else {
				if err := s.syncFeedback(ctx, tx, feedback); err != nil {
					return err
				}
				progress.Cursor = feedback.ID
			}
		default:
			return errors.New("invalid recommendation sync state")
		}
		data, err := json.Marshal(progress)
		if err != nil {
			return err
		}
		return tx.Model(&state).Updates(map[string]any{"value": string(data), "updated_at": time.Now().UTC()}).Error
	})
}
