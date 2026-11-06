package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/auth"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestAdminSourceSettingsDoNotExposeSecrets(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	cfg := config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}
	if err := store.Save(config.Config{Installed: true, Database: cfg}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.NewService(db).Register(context.Background(), fmt.Sprintf("admin-api-%d@example.com", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Where("id = ?", user.ID).Delete(&auth.User{})
	if err := db.Model(&user).Update("is_admin", true).Error; err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.NewService(db).Login(context.Background(), user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM settings WHERE name = ?", "sources")
	router := NewRouter(setup.NewService(store, setupBackend{}), provider)
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/sources", bytes.NewBufferString(`{"fanza_api_id":"sensitive-id","fanza_affiliate_id":"sensitive-affiliate","jable_cookie":"session=sensitive-jable","javlibrary_cookie":"session=sensitive-library"}`))
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("sensitive")) {
		t.Fatalf("update: %d %s", response.Code, response.Body.String())
	}
	read := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/sources", nil)
	read.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, read)
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte("sensitive")) || !bytes.Contains(response.Body.Bytes(), []byte(`"jable_cookie_configured":true`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"javlibrary_cookie_configured":true`)) {
		t.Fatalf("read: %d %s", response.Code, response.Body.String())
	}
}
