package database

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
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
		if err := db.Exec("DELETE FROM goose_db_version WHERE version_id = ?", 2).Error; err != nil {
			t.Fatal(err)
		}
		if err := Migrate(context.Background(), db, driver); err != nil {
			t.Fatalf("migration did not recover after the index was created: %v", err)
		}
	}
	for _, name := range []string{"users", "api_sessions", "movies", "actresses", "source_items", "source_pages", "movie_cast", "settings", "search_cooldowns"} {
		if !db.Migrator().HasTable(name) {
			t.Fatalf("missing table %s", name)
		}
	}
}
