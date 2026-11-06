package database

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
)

func TestInstallationLockIsExclusiveAcrossConnections(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	err = WithInstallationLock(context.Background(), db, driver, func() error {
		return WithInstallationLock(context.Background(), db, driver, func() error { return nil })
	})
	if !errors.Is(err, ErrInstallationLocked) {
		t.Fatalf("expected lock conflict, got %v", err)
	}
	if err := WithInstallationLock(context.Background(), db, driver, func() error { return nil }); err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
}
