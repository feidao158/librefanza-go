package settings

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/sources"
	"github.com/libredmm/librefanza/backend/internal/tasks"
)

func TestSourceSettingsPreserveSecrets(t *testing.T) {
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
	service := NewService(db)
	defer db.Exec("DELETE FROM settings WHERE name = ?", "sources")
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{FanzaAPIID: "secret-api", FanzaAffiliateID: "secret-affiliate"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{FC2URL: "https://example.com/"}); err != nil {
		t.Fatal(err)
	}
	stored, err := service.Sources(context.Background())
	if err != nil || stored.FanzaAPIID != "secret-api" || stored.FanzaAffiliateID != "secret-affiliate" || stored.FC2URL != "https://example.com/" {
		t.Fatalf("settings not preserved: %v", err)
	}
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{ProxyURL: "http://localhost:8080"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{ClearProxy: true}); err != nil {
		t.Fatal(err)
	}
	stored, err = service.Sources(context.Background())
	if err != nil || stored.ProxyURL != "" {
		t.Fatalf("proxy not cleared: %v", err)
	}
}

func TestRedisPasswordCanBeCleared(t *testing.T) {
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
	service := NewService(db)
	defer db.Exec("DELETE FROM settings WHERE name = ?", "redis")
	if err := service.UpdateRedis(context.Background(), config.Redis{Address: "localhost:6379", Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateRedis(context.Background(), config.Redis{ClearPassword: true}); err != nil {
		t.Fatal(err)
	}
	stored, err := service.Redis(context.Background())
	if err != nil || stored.Password != "" {
		t.Fatalf("password not cleared: %v", err)
	}
}

func TestScheduleSettingsValidateExpressions(t *testing.T) {
	settings := tasks.DefaultScheduleSettings()
	settings.Fanza.Cron = "not a cron expression"
	if err := tasks.ValidateScheduleSettings(settings); err == nil {
		t.Fatal("invalid expression accepted")
	}
}
