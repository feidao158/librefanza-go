package database

import (
	"context"
	"errors"
	"sync"

	"github.com/libredmm/librefanza/backend/internal/config"
	"gorm.io/gorm"
)

type Provider struct {
	store *config.Store
	mu    sync.Mutex
	db    *gorm.DB
}

func NewProvider(store *config.Store) *Provider { return &Provider{store: store} }

func (p *Provider) Database(ctx context.Context) (*gorm.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db != nil {
		return p.db, nil
	}
	cfg, found, err := p.store.Load()
	if err != nil {
		return nil, err
	}
	if !found || !cfg.Installed {
		return nil, errors.New("application is not installed")
	}
	p.db, err = Open(ctx, cfg.Database)
	if err == nil {
		migrateErr := WithInstallationLock(ctx, p.db, cfg.Database.Driver, func() error { return Migrate(ctx, p.db, cfg.Database.Driver) })
		if migrateErr != nil {
			sqlDB, _ := p.db.DB()
			_ = sqlDB.Close()
			p.db = nil
			return nil, errors.New("database migration failed")
		}
	}
	return p.db, err
}

func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.db == nil {
		return nil
	}
	db, err := p.db.DB()
	if err != nil {
		return err
	}
	p.db = nil
	return db.Close()
}
