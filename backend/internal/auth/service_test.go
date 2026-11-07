package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

func TestAuthenticationRenewsPersistentTokenAgainstDatabase(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, err := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	if err != nil {
		t.Fatal(err)
	}
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
	email := fmt.Sprintf("user-%d@example.com", time.Now().UnixNano())
	if _, err := service.Register(context.Background(), email, "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Login(context.Background(), email, "wrong-password"); err == nil {
		t.Fatal("incorrect password was accepted")
	}
	tokens, err := service.Login(context.Background(), email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.ExpiresIn != 30*24*60*60 {
		t.Fatalf("unexpected login lifetime: %d", tokens.ExpiresIn)
	}
	oldExpiry := time.Now().UTC().Add(time.Hour)
	if err := db.Model(&session{}).Where("access_hash = ?", tokenHash(tokens.AccessToken)).Update("access_expires_at", oldExpiry).Error; err != nil {
		t.Fatal(err)
	}
	user, err := service.Authenticate(context.Background(), tokens.AccessToken)
	if err != nil || user.Email != email || user.IsAdmin {
		t.Fatalf("authentication: user=%+v err=%v", user, err)
	}
	var renewed session
	if err := db.Where("access_hash = ?", tokenHash(tokens.AccessToken)).First(&renewed).Error; err != nil || renewed.AccessExpiresAt.Before(time.Now().UTC().Add(29*24*time.Hour)) {
		t.Fatalf("session was not renewed: %+v, %v", renewed, err)
	}
	if _, err := service.Authenticate(context.Background(), tokens.AccessToken); err != nil {
		t.Fatalf("same token was invalidated by another request: %v", err)
	}
	passiveExpiry := time.Now().UTC().Add(time.Hour)
	if err := db.Model(&session{}).Where("access_hash = ?", tokenHash(tokens.AccessToken)).Update("access_expires_at", passiveExpiry).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateWithActivity(context.Background(), tokens.AccessToken, false); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("access_hash = ?", tokenHash(tokens.AccessToken)).First(&renewed).Error; err != nil || renewed.AccessExpiresAt.After(passiveExpiry.Add(time.Second)) {
		t.Fatal("passive request renewed session")
	}
	if err := db.Model(&session{}).Where("access_hash = ?", tokenHash(tokens.AccessToken)).Update("access_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), tokens.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("expired token was accepted")
	}
	if err := service.Logout(context.Background(), tokens.AccessToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), tokens.AccessToken); err == nil {
		t.Fatal("logged-out token was accepted")
	}
}

func TestPasswordResetTokenIsSingleUse(t *testing.T) {
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
	email := fmt.Sprintf("reset-%d@example.com", time.Now().UnixNano())
	user, err := service.Register(context.Background(), email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&user)
	session, err := service.Login(context.Background(), email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	token, err := service.CreatePasswordReset(context.Background(), email)
	if err != nil || token == "" {
		t.Fatalf("create reset: %v", err)
	}
	if err := service.ResetPassword(context.Background(), token, "new-password123"); err != nil {
		t.Fatal(err)
	}
	if err := service.ResetPassword(context.Background(), token, "another-password"); err == nil {
		t.Fatal("reset token was reused")
	}
	if _, err := service.Login(context.Background(), email, "new-password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(context.Background(), session.AccessToken); err == nil {
		t.Fatal("old session remained valid")
	}
}

func TestRegistrationRejectsCaseVariantDuplicate(t *testing.T) {
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
	email := fmt.Sprintf("Case-%d@example.com", time.Now().UnixNano())
	service := NewService(db)
	user, err := service.Register(context.Background(), email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&user)
	if _, err := service.Register(context.Background(), strings.ToUpper(email), "password123"); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate: %v", err)
	}
}
