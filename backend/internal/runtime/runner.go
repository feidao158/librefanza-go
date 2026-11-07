package runtime

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/ingest"
	"github.com/libredmm/librefanza/backend/internal/logbook"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
)

type Runner struct {
	provider *database.Provider
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	manager  *tasks.Manager
}

func NewRunner(parent context.Context, provider *database.Provider) *Runner {
	ctx, cancel := context.WithCancel(parent)
	return &Runner{provider: provider, ctx: ctx, cancel: cancel}
}

func (r *Runner) Manager(ctx context.Context) (*tasks.Manager, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if r.manager != nil {
		if r.manager.Running() {
			return r.manager, nil
		}
		r.manager.Close()
		r.manager = nil
	}
	db, err := r.provider.Database(ctx)
	if err != nil {
		return nil, err
	}
	var manager *tasks.Manager
	search := func(ctx context.Context, id string) (bool, error) {
		pattern := os.Getenv("BLACKHOLE_PATTERN")
		if pattern != "" {
			excluded, err := regexp.Compile("(?i)" + pattern)
			if err != nil {
				return false, errors.New("invalid exclusion pattern")
			}
			if excluded.MatchString(id) {
				return false, nil
			}
		}
		sourceSettings, err := settings.NewService(db).Sources(ctx)
		if err != nil {
			return false, err
		}
		providers, err := sources.NewProviders(sourceSettings)
		if err != nil {
			return false, err
		}
		return sources.Search(ctx, id, providers, ingest.NewService(db), func(source string, err error) {
			manager.RecordSourceFailure(ctx, source, err)
		})
	}
	err = database.WithInstallationLock(ctx, db, db.Dialector.Name(), func() error {
		var err error
		manager, err = tasks.NewManager(db, search)
		return err
	})
	if err != nil {
		return nil, err
	}
	maintain := func(ctx context.Context, kind string) error {
		cfg, err := settings.NewService(db).Sources(ctx)
		if err != nil {
			return err
		}
		sink := ingest.NewService(db)
		switch kind {
		case "fanza_crawl":
			return sources.CrawlFanza(ctx, cfg, 3, sink)
		case "mgstage_crawl":
			return sources.CrawlMGStage(ctx, cfg, sink)
		case "actress_crawl":
			var count int64
			if err := db.WithContext(ctx).Model(&catalog.Actress{}).Count(&count).Error; err != nil {
				return err
			}
			offset := int(count) - 999
			return sources.CrawlActresses(ctx, cfg, offset, func(record sources.ActressRecord) error { return sink.SaveActress(ctx, record) })
		case "rebuild_movies":
			return sink.RebuildMovies(ctx)
		default:
			return errors.New("invalid maintenance task")
		}
	}
	manager.ConfigureCrawls(func(ctx context.Context, request sources.CrawlRequest, progress sources.CrawlProgress, save func(sources.CrawlProgress) error) error {
		cfg, err := settings.NewService(db).Sources(ctx)
		if err != nil {
			return err
		}
		source, err := sources.NewCrawlSource(cfg, request.Source)
		if err != nil {
			return err
		}
		return sources.RunCrawl(ctx, request, source, ingest.NewService(db), progress, save)
	})
	if err := manager.ConfigureMetadata(func(ctx context.Context, id, source string) (bool, error) {
		cfg, err := settings.NewService(db).Sources(ctx)
		if err != nil {
			return false, err
		}
		providers, err := sources.NewProviders(cfg)
		if err != nil {
			return false, err
		}
		for _, provider := range providers {
			if named, ok := provider.(interface{ SourceName() string }); ok && named.SourceName() == source {
				return sources.Search(ctx, id, []sources.Provider{provider}, ingest.NewService(db))
			}
		}
		return false, errors.New("invalid metadata source")
	}, func(ctx context.Context, item catalog.SourceItem) error {
		err := ingest.NewService(db).ReparseSaved(ctx, item)
		if errors.Is(err, ingest.ErrStoredSourceInvalid) {
			return tasks.ErrStoredSourceInvalid
		}
		return err
	}); err != nil {
		return nil, err
	}
	if err := manager.ConfigureMaintenance(func(ctx context.Context) (tasks.ScheduleSettings, error) {
		return settings.NewService(db).Schedules(ctx)
	}, map[string]func(context.Context) error{
		"fanza_crawl":    func(ctx context.Context) error { return maintain(ctx, "fanza_crawl") },
		"mgstage_crawl":  func(ctx context.Context) error { return maintain(ctx, "mgstage_crawl") },
		"actress_crawl":  func(ctx context.Context) error { return maintain(ctx, "actress_crawl") },
		"rebuild_movies": func(ctx context.Context) error { return maintain(ctx, "rebuild_movies") },
	}); err != nil {
		return nil, err
	}
	if err := manager.EnsureBackfill(ctx); err != nil {
		return nil, err
	}
	if err := manager.Start(r.ctx); err != nil {
		return nil, err
	}
	r.manager = manager
	return manager, nil
}

func (r *Runner) Run() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	logs := logbook.New(nil, os.Stdout)
	lastError := ""
	var nextCleanup time.Time
	for {
		manager, err := r.Manager(r.ctx)
		if err != nil && r.ctx.Err() == nil && !errors.Is(err, database.ErrNotInstalled) {
			if err.Error() != lastError {
				entry := logbook.Entry{Level: "error", Event: "worker_start_failed"}
				logbook.ApplyFailure(&entry, err)
				logs.Record(r.ctx, entry)
				lastError = err.Error()
			}
		} else if err == nil {
			if lastError != "" {
				logs.Record(r.ctx, logbook.Entry{Level: "info", Event: "worker_started"})
				lastError = ""
			}
			if time.Now().After(nextCleanup) {
				cleanup, cancel := context.WithTimeout(r.ctx, 15*time.Second)
				if err := manager.CleanLogs(cleanup); err != nil {
					logs.Record(r.ctx, logbook.Entry{Level: "error", Event: "log_cleanup_failed", Reason: "storage"})
				}
				cancel()
				nextCleanup = time.Now().Add(24 * time.Hour)
			}
		}
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) Close() {
	r.cancel()
	r.mu.Lock()
	manager := r.manager
	r.manager = nil
	r.mu.Unlock()
	if manager != nil {
		manager.Close()
	}
}
