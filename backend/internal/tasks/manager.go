package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SearchFunc func(context.Context, string) (bool, error)

var ErrInvalidID = errors.New("invalid product ID")
var ErrJobNotFound = errors.New("job not found")

type SearchArgs struct {
	ID    string `json:"id"`
	Force bool   `json:"force"`
}

type Result struct {
	Found  bool `json:"found"`
	Cached bool `json:"cached"`
}

type Status struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Result *Result `json:"result,omitempty"`
}

type cooldown struct {
	NormalizedID string `gorm:"primaryKey"`
	SearchedAt   time.Time
}

func (cooldown) TableName() string { return "search_cooldowns" }

type Manager struct {
	db      *gorm.DB
	storage *jobs.GormStorage
	queue   *jobs.Queue
	worker  *jobs.Worker
	logs    *logbook.Recorder
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewManager(db *gorm.DB, search SearchFunc) (*Manager, error) {
	return newManager(db, search)
}

func newManager(db *gorm.DB, search SearchFunc, overrides ...jobs.WorkerOption) (*Manager, error) {
	storage := jobs.NewGormStorage(db)
	if err := storage.Migrate(context.Background()); err != nil {
		return nil, err
	}
	queue := jobs.New(storage)
	manager := &Manager{db: db, storage: storage, queue: queue, logs: logbook.New(db, os.Stdout)}
	queue.UseExecutionMiddleware(manager.logExecution)
	queue.Register("movie_search", func(ctx context.Context, args SearchArgs) (Result, error) {
		return protectTask(func() (Result, error) {
			if !args.Force {
				var previous cooldown
				err := db.WithContext(ctx).Where("normalized_id = ?", args.ID).First(&previous).Error
				if err == nil && previous.SearchedAt.After(time.Now().UTC().Add(-24*time.Hour)) {
					return Result{Cached: true}, nil
				}
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return Result{}, err
				}
			}
			found, err := search(ctx, args.ID)
			if err != nil {
				return Result{}, err
			}
			entry := cooldown{NormalizedID: args.ID, SearchedAt: time.Now().UTC()}
			if err := db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "normalized_id"}}, DoUpdates: clause.AssignmentColumns([]string{"searched_at"})}).Create(&entry).Error; err != nil {
				return Result{}, err
			}
			return Result{Found: found}, nil
		})
	})
	options := []jobs.WorkerOption{
		jobs.WorkerQueue("default", jobs.Concurrency(4)),
		jobs.WithPollInterval(200 * time.Millisecond),
		jobs.WithScheduler(true),
		jobs.WithDrainTimeout(5 * time.Second),
		jobs.WithStaleLockInterval(10 * time.Second),
		jobs.WithStaleLockAge(2 * time.Minute),
	}
	worker := jobs.NewWorker(queue, append(options, overrides...)...)
	manager.worker = worker
	return manager, nil
}

func (m *Manager) Start(parent context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		return errors.New("worker already started")
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancel = cancel
	done := make(chan struct{})
	m.done = done
	go func() {
		defer close(done)
		if err := m.worker.Start(ctx); err != nil && ctx.Err() == nil {
			m.logs.Record(ctx, logbook.Entry{Level: "error", Event: "worker_stopped", Reason: "unknown"})
		}
	}()
	return nil
}

func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel == nil || m.done == nil {
		return false
	}
	select {
	case <-m.done:
		return false
	default:
		return true
	}
}

func (m *Manager) Close() {
	m.mu.Lock()
	cancel, done := m.cancel, m.done
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
		m.mu.Lock()
		if m.done == done {
			m.cancel = nil
		}
		m.mu.Unlock()
	}
}

func (m *Manager) Enqueue(ctx context.Context, value string, force bool) (string, error) {
	if len(value) > 128 || !catalog.IsProductID(value) {
		return "", ErrInvalidID
	}
	for _, char := range value {
		if char > 127 {
			return "", ErrInvalidID
		}
	}
	id := catalog.NormalizeProductID(value)
	key := "movie_search:" + id
	if force {
		key += ":force"
	}
	jobID, err := m.queue.Enqueue(ctx, "movie_search", SearchArgs{ID: id, Force: force}, jobs.Unique(key), jobs.Retries(3), jobs.Timeout(10*time.Minute))
	if errors.Is(err, jobs.ErrDuplicateJob) {
		var existing jobs.Job
		err = m.db.WithContext(ctx).Where("unique_key = ? AND status IN ?", key, []string{"pending", "running", "retrying", "waiting", "paused"}).Order("created_at DESC").First(&existing).Error
		if err == nil {
			return existing.ID.String(), nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("enqueue search: %w", err)
	}
	return jobID.String(), nil
}

func (m *Manager) Get(ctx context.Context, value string) (Status, error) {
	id, err := jobs.ParseUUID(value)
	if err != nil {
		return Status{}, ErrJobNotFound
	}
	job, err := m.storage.GetJob(ctx, id)
	if err != nil {
		return Status{}, err
	}
	if job == nil || job.Type != "movie_search" {
		return Status{}, ErrJobNotFound
	}
	status := Status{ID: job.ID.String(), Status: string(job.Status)}
	if len(job.Result) > 0 {
		var result Result
		var args SearchArgs
		if err := json.Unmarshal(job.Result, &result); err != nil {
			return Status{}, err
		}
		if err := json.Unmarshal(job.Args, &args); err != nil || args.ID == "" {
			return Status{}, errors.New("invalid search job arguments")
		}
		var visible int64
		if err := m.db.WithContext(ctx).Model(&catalog.Movie{}).Where("normalized_id = ? AND is_hidden = ?", args.ID, false).Count(&visible).Error; err != nil {
			return Status{}, err
		}
		result.Found = visible > 0
		status.Result = &result
	}
	return status, nil
}
