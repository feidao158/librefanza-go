package cache

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Manager struct {
	provider *database.Provider
	mu       sync.Mutex
	config   config.Redis
	client   *redis.Client
}

type Handle struct {
	client *redis.Client
	key    string
}

func NewManager(provider *database.Provider) *Manager { return &Manager{provider: provider} }

func (m *Manager) Get(ctx context.Context, key string) ([]byte, *Handle) {
	db, err := m.provider.Database(ctx)
	if err != nil {
		return nil, nil
	}
	cfg, err := settings.NewService(db).Redis(ctx)
	if err != nil || !cfg.Enabled {
		return nil, nil
	}
	var version struct{ Value string }
	err = db.WithContext(ctx).Table("settings").Select("value").Where("name = ?", "catalog_version").First(&version).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	m.mu.Lock()
	if m.client == nil || m.config != cfg {
		if m.client != nil {
			m.client.Close()
		}
		options := &redis.Options{Addr: cfg.Address, Username: cfg.Username, Password: cfg.Password, DB: cfg.Database, MaxRetries: -1, DialTimeout: 300 * time.Millisecond, ReadTimeout: 300 * time.Millisecond, WriteTimeout: 300 * time.Millisecond}
		if cfg.TLS {
			options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		m.client = redis.NewClient(options)
		m.config = cfg
	}
	client := m.client
	m.mu.Unlock()
	handle := &Handle{client: client, key: "librefanza:catalog:" + version.Value + ":" + key}
	readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	value, err := client.Get(readCtx, handle.key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, handle
	}
	if err != nil {
		return nil, nil
	}
	return value, handle
}

func (h *Handle) Set(ctx context.Context, value []byte) {
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	_ = h.client.Set(ctx, h.key, value, time.Minute).Err()
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.client != nil {
		_ = m.client.Close()
		m.client = nil
	}
}

func BumpVersion(db *gorm.DB) error {
	row := struct {
		Name      string `gorm:"primaryKey"`
		Value     string
		UpdatedAt time.Time
	}{Name: "catalog_version", Value: uuid.NewString(), UpdatedAt: time.Now().UTC()}
	return db.Table("settings").Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name"}}, DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"})}).Create(&row).Error
}
