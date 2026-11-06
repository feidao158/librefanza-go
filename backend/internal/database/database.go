package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/libredmm/librefanza/backend/internal/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func ConnectionString(cfg config.Database) (string, error) {
	if (cfg.Driver != "mysql" && cfg.Driver != "postgres") || cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || cfg.Name == "" || cfg.User == "" {
		return "", errors.New("invalid database configuration")
	}
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if cfg.Driver == "mysql" {
		connection := mysqldriver.NewConfig()
		connection.User = cfg.User
		connection.Passwd = cfg.Password
		connection.Net = "tcp"
		connection.Addr = address
		connection.DBName = cfg.Name
		connection.ParseTime = true
		connection.Loc = time.UTC
		connection.Params = map[string]string{"charset": "utf8mb4"}
		if cfg.TLS {
			connection.TLSConfig = "true"
		}
		return connection.FormatDSN(), nil
	}
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
	var dialector gorm.Dialector
	if cfg.Driver == "mysql" {
		dialector = mysql.Open(dsn)
	} else {
		dialector = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
