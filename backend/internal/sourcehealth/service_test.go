package sourcehealth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func TestCooldownClassification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		failures int
		want     time.Duration
	}{
		{"network first", &sources.RequestError{Reason: "dns"}, 1, 0},
		{"network threshold", &sources.RequestError{Reason: "dns"}, 3, 5 * time.Minute},
		{"network backoff", &sources.RequestError{Reason: "dns"}, 4, 10 * time.Minute},
		{"forbidden", &sources.RequestError{Reason: "http", Status: 403}, 1, 30 * time.Minute},
		{"forbidden backoff", &sources.RequestError{Reason: "http", Status: 403}, 2, time.Hour},
		{"challenge", errors.New("source requires verification"), 1, 30 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cooldown(tc.err, tc.failures); got != tc.want {
				t.Fatalf("冷却时间 = %v，期望 %v", got, tc.want)
			}
		})
	}
}

func TestRateLimitDeadlineIsPersisted(t *testing.T) {
	deadline := time.Now().UTC().Add(2 * time.Hour)
	delay := cooldown(&sources.RequestError{Reason: "http", Status: 429, RetryAt: deadline}, 1)
	if delay < 119*time.Minute || delay > 121*time.Minute {
		t.Fatalf("未遵循来源限速恢复时间: %v", delay)
	}
}

func TestHealthPersistsCooldownAndRecovers(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	db, err := database.Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := database.Migrate(context.Background(), db, driver); err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf("test-%d", time.Now().UnixNano())
	defer db.Where("source = ?", source).Delete(&State{})
	service := New(db)
	if until, err := service.Begin(context.Background(), source); err != nil || !until.IsZero() {
		t.Fatalf("初始状态错误: %v %v", until, err)
	}
	report, err := service.End(context.Background(), source, &sources.RequestError{Reason: "http", Status: 403})
	if err != nil || !report {
		t.Fatalf("未记录来源故障: %v", err)
	}
	if until, err := New(db).Begin(context.Background(), source); err != nil || !until.After(time.Now()) {
		t.Fatalf("跨实例冷却无效: %v %v", until, err)
	}
	if err := db.Model(&State{}).Where("source = ?", source).Update("next_retry", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if until, err := service.Begin(context.Background(), source); err != nil || !until.IsZero() {
		t.Fatalf("恢复探测未执行: %v %v", until, err)
	}
	if until, err := New(db).Begin(context.Background(), source); err != nil || !until.After(time.Now()) {
		t.Fatalf("并发探测未受租约限制: %v %v", until, err)
	}
	if _, err := service.End(context.Background(), source, nil); err != nil {
		t.Fatal(err)
	}
	if until, err := service.Begin(context.Background(), source); err != nil || !until.IsZero() {
		t.Fatalf("恢复后仍处于冷却: %v %v", until, err)
	}
}
