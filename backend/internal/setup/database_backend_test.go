package setup

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"golang.org/x/crypto/bcrypt"
)

func TestDatabaseBackendPrepareIsIdempotent(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, err := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}
	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	email := fmt.Sprintf("setup-%d@example.com", time.Now().UnixNano())
	defer db.Exec("DELETE FROM users WHERE email = ?", email)
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	backend := NewDatabaseBackend()
	for i := 0; i < 2; i++ {
		if err := backend.Prepare(context.Background(), cfg, Redis{}, email, string(hash)); err != nil {
			t.Fatalf("prepare %d: %v", i, err)
		}
	}
	var count int64
	if err := db.Table("users").Where("email = ? AND is_admin = ?", email, true).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("admin count=%d err=%v", count, err)
	}
}
