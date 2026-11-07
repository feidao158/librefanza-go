package database

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/pressly/goose/v3"
)

func TestMigrationsAgainstDatabase(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, err := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	for i := 0; i < 2; i++ {
		if err := Migrate(context.Background(), db, driver); err != nil {
			t.Fatalf("migration %d: %v", i, err)
		}
	}
	if driver == "mysql" {
		if err := db.Exec("DELETE FROM goose_db_version WHERE version_id IN ?", []int{2, 3, 4}).Error; err != nil {
			t.Fatal(err)
		}
		files, err := fs.Sub(migrations, "migrations/mysql")
		if err != nil {
			t.Fatal(err)
		}
		provider, err := goose.NewProvider(goose.Dialect(driver), sqlDB, files, goose.WithAllowOutofOrder(true))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Up(context.Background()); err != nil {
			t.Fatalf("migration did not recover after the index was created: %v", err)
		}
		if err := Migrate(context.Background(), db, driver); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"users", "api_sessions", "movies", "actresses", "source_items", "source_pages", "movie_cast", "settings", "search_cooldowns", "source_crawl_progress", "application_logs"} {
		if !db.Migrator().HasTable(name) {
			t.Fatalf("missing table %s", name)
		}
	}
}

func TestScheduleMigrationKeepsDisabledAndActressSettings(t *testing.T) {
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
	files, err := fs.Sub(migrations, "migrations/"+driver)
	if err != nil {
		t.Fatal(err)
	}
	body, err := fs.ReadFile(files, "00006_half_hour_schedules.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(body), "-- +goose Down")
	var old string
	result := db.Raw("SELECT value FROM settings WHERE name = ?", "schedules").Scan(&old)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	defer func() {
		if old == "" {
			db.Exec("DELETE FROM settings WHERE name = ?", "schedules")
		} else {
			db.Exec("UPDATE settings SET value = ? WHERE name = ?", old, "schedules")
		}
	}()
	value := `{"timezone":"Asia/Shanghai","actress":{"enabled":true,"cron":"1 2 * * *"},"fanza":{"enabled":true,"cron":"0 1 * * *"},"jable":{"enabled":false,"cron":"0 3 * * *"},"avmoo":{"enabled":true,"cron":"0 4 * * *"}}`
	if old == "" {
		err = db.Exec("INSERT INTO settings (name, value, updated_at) VALUES (?, ?, ?)", "schedules", value, time.Now().UTC()).Error
	} else {
		err = db.Exec("UPDATE settings SET value = ? WHERE name = ?", value, "schedules").Error
	}
	if err != nil {
		t.Fatal(err)
	}
	up := strings.TrimPrefix(parts[0], "-- +goose Up")
	for _, statement := range strings.Split(up, ";") {
		if strings.TrimSpace(statement) != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	var saved string
	if err := db.Raw("SELECT value FROM settings WHERE name = ?", "schedules").Scan(&saved).Error; err != nil {
		t.Fatal(err)
	}
	var schedule map[string]json.RawMessage
	if err := json.Unmarshal([]byte(saved), &schedule); err != nil {
		t.Fatal(err)
	}
	var fanza, jable, actress, avmoo struct {
		Cron    string `json:"cron"`
		Enabled bool   `json:"enabled"`
	}
	for key, target := range map[string]any{"fanza": &fanza, "jable": &jable, "actress": &actress, "avmoo": &avmoo} {
		if err := json.Unmarshal(schedule[key], target); err != nil {
			t.Fatal(err)
		}
	}
	if fanza.Cron != "*/30 * * * *" || avmoo.Cron != "*/30 * * * *" || jable.Enabled || jable.Cron != "0 3 * * *" || actress.Cron != "1 2 * * *" || string(schedule["timezone"]) != `"Asia/Shanghai"` {
		t.Fatalf("unexpected schedules: %s", saved)
	}
}
