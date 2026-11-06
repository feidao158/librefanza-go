package database

import (
	"context"
	sqldriver "database/sql/driver"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrInstallationLocked = errors.New("database installation is locked")

func WithInstallationLock(ctx context.Context, db *gorm.DB, driver string, fn func() error) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var locked bool
	var acquire, release string
	switch driver {
	case "postgres":
		acquire, release = "SELECT pg_try_advisory_lock(hashtext('librefanza/setup'))", "SELECT pg_advisory_unlock(hashtext('librefanza/setup'))"
	case "mysql":
		acquire, release = "SELECT GET_LOCK(CONCAT(DATABASE(), ':librefanza:setup'), 0)", "SELECT RELEASE_LOCK(CONCAT(DATABASE(), ':librefanza:setup'))"
	default:
		return errors.New("unsupported database driver")
	}
	if err := conn.QueryRowContext(ctx, acquire).Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return ErrInstallationLocked
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(releaseCtx, release); err != nil {
			_ = conn.Raw(func(any) error { return sqldriver.ErrBadConn })
		}
	}()
	return fn()
}
