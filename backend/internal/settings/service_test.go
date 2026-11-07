package settings

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/sourcehealth"
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
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{FanzaAPIID: "secret-api", FanzaAffiliateID: "secret-affiliate", JableCookie: "session=secret-jable", JavLibraryCookie: "session=secret-library"}); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{FC2URL: "https://example.com/"}); err != nil {
		t.Fatal(err)
	}
	stored, err := service.Sources(context.Background())
	if err != nil || stored.FanzaAPIID != "secret-api" || stored.FanzaAffiliateID != "secret-affiliate" || stored.FC2URL != "https://example.com/" || stored.JableCookie != "session=secret-jable" || stored.JavLibraryCookie != "session=secret-library" {
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
	if err := service.UpdateSources(context.Background(), sources.ClientSettings{ClearJableCookie: true, ClearJavLibraryCookie: true}); err != nil {
		t.Fatal(err)
	}
	stored, err = service.Sources(context.Background())
	if err != nil || stored.JableCookie != "" || stored.JavLibraryCookie != "" {
		t.Fatal("来源会话未清除")
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

func TestVerifiedSourceAutoScheduleRespectsManualDisable(t *testing.T) {
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
	defer db.Exec("DELETE FROM settings WHERE name = ?", "schedules")
	defer db.Where("source = ?", "javdb").Delete(&sourcehealth.State{})
	if err := db.Create(&sourcehealth.State{Source: "javdb", Status: "available", Verified: true, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewService(db)
	value, err := service.Schedules(context.Background())
	if err != nil || !value.JavDB.Enabled || !value.JavDB.Auto {
		t.Fatalf("已验证来源未自动启用: %+v %v", value.JavDB, err)
	}
	value.JavDB.Enabled, value.JavDB.Auto = false, false
	if err := service.UpdateSchedules(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	value, err = service.Schedules(context.Background())
	if err != nil || value.JavDB.Enabled || value.JavDB.Auto {
		t.Fatalf("人工禁用被覆盖: %+v %v", value.JavDB, err)
	}
}
