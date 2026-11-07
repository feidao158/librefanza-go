package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
)

func TestSourceRateLimitIsSharedAcrossInstances(t *testing.T) {
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
	if err := Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("test-%d", time.Now().UnixNano())
	defer db.Where("source = ?", name).Delete(&sourceRequestLimit{})
	first := newSourceRateLimiter(db, 100*time.Millisecond)
	second := newSourceRateLimiter(db, 100*time.Millisecond)
	if err := first.Wait(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := second.Wait(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 80*time.Millisecond {
		t.Fatal("different limiters did not share their interval")
	}
	if err := first.Pause(context.Background(), name, time.Now().Add(180*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	if err := second.Wait(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 150*time.Millisecond {
		t.Fatal("retry-after pause was not shared")
	}
}
