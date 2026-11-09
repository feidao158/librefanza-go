package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ConnectionString(cfg config.Database) (string, error) {
	if cfg.Driver != "postgres" || cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.Name == "" || cfg.User == "" {
		return "", errors.New("invalid database configuration")
	}
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	connection := url.URL{Scheme: "postgres", User: url.UserPassword(cfg.User, cfg.Password), Host: address, Path: "/" + cfg.Name}
	values := connection.Query()
	if cfg.TLS {
		values.Set("sslmode", "verify-full")
	} else {
		values.Set("sslmode", "disable")
	}
	connection.RawQuery = values.Encode()
	return connection.String(), nil
}

func Open(ctx context.Context, cfg config.Database) (*gorm.DB, error) {
	dsn, err := ConnectionString(cfg)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, errors.New("cannot connect to database")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.New("cannot access database connection")
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("database connection test failed")
	}
	return db, nil
}
