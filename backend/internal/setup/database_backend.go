package setup

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"time"

	jobs "github.com/jdziat/simple-durable-jobs/v4"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DatabaseBackend struct{}

func NewDatabaseBackend() *DatabaseBackend { return &DatabaseBackend{} }

func (b *DatabaseBackend) CheckDatabase(ctx context.Context, cfg config.Database) error {
	db, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	for _, table := range []string{"users", "api_sessions", "password_reset_tokens", "movies", "actresses", "source_items", "source_pages", "movie_cast", "settings", "search_cooldowns", "goose_db_version", "jobs"} {
		if db.Migrator().HasTable(table) {
			return errors.New("database contains application tables")
		}
	}
	return nil
}

func (b *DatabaseBackend) CheckRedis(ctx context.Context, cfg Redis) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Address == "" || cfg.Database < 0 {
		return errors.New("invalid redis configuration")
	}
	options := &redis.Options{Addr: cfg.Address, Username: cfg.Username, Password: cfg.Password, DB: cfg.Database, DialTimeout: 5 * time.Second}
	if cfg.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	client := redis.NewClient(options)
	defer client.Close()
	if err := client.Ping(ctx).Err(); err != nil {
		return errors.New("redis connection test failed")
	}
	return nil
}

type initialUser struct {
	ID           uint64
	Email        string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (initialUser) TableName() string { return "users" }

type initialSetting struct {
	Name      string
	Value     string
	UpdatedAt time.Time
}

func (initialSetting) TableName() string { return "settings" }

func (b *DatabaseBackend) Prepare(ctx context.Context, cfg config.Database, cache Redis, email, passwordHash string) error {
	db, err := database.Open(ctx, cfg)
	if err != nil {
		return err
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	return database.WithInstallationLock(ctx, db, cfg.Driver, func() error {
		if err := database.Migrate(ctx, db, cfg.Driver); err != nil {
			return errors.New("database migration failed")
		}
		if err := jobs.NewGormStorage(db).Migrate(ctx); err != nil {
			return errors.New("job storage migration failed")
		}
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var admin initialUser
			err := tx.Where("is_admin = ?", true).First(&admin).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Create(&initialUser{Email: email, PasswordHash: passwordHash, IsAdmin: true}).Error; err != nil {
					return errors.New("could not create initial administrator")
				}
			} else if err != nil || admin.Email != email || admin.PasswordHash != passwordHash {
				return errors.New("existing administrator does not match setup")
			}
			value, err := json.Marshal(cache)
			if err != nil {
				return err
			}
			setting := initialSetting{Name: "redis", Value: string(value), UpdatedAt: time.Now().UTC()}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&setting).Error; err != nil {
				return errors.New("could not save initial settings")
			}
			return nil
		})
	})
}
