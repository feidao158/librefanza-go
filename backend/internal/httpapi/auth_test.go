package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/libredmm/librefanza/backend/internal/mailer"
	"github.com/libredmm/librefanza/backend/internal/settings"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

func TestAuthAPIUsesSingleToken(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, err := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(t.TempDir())
	if err := store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}}); err != nil {
		t.Fatal(err)
	}
	provider := database.NewProvider(store)
	defer provider.Close()
	router := NewRouter(setup.NewService(store, setupBackend{}), provider)
	email := fmt.Sprintf("api-%d@example.com", time.Now().UnixNano())
	body := []byte(fmt.Sprintf(`{"email":%q,"password":"password123","client":"mobile"}`, email))
	registration := httptest.NewRecorder()
	router.ServeHTTP(registration, httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body)))
	if registration.Code != http.StatusCreated {
		t.Fatalf("register: %d %s", registration.Code, registration.Body.String())
	}
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions", bytes.NewReader(body)))
	if login.Code != http.StatusCreated {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	var payload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &payload); err != nil || payload.Data.AccessToken == "" || bytes.Contains(login.Body.Bytes(), []byte("refresh_token")) || len(login.Result().Cookies()) != 0 {
		t.Fatalf("invalid token response: %v", err)
	}
	me := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+payload.Data.AccessToken)
	router.ServeHTTP(me, req)
	if me.Code != http.StatusOK || !bytes.Contains(me.Body.Bytes(), []byte(email)) {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
}

func TestPasswordResetDoesNotRevealWhetherAccountExists(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	_ = store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}})
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.NewService(db).Register(context.Background(), fmt.Sprintf("reset-api-%d@example.com", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&user)
	smtp := mailer.Settings{Enabled: true, Host: "127.0.0.1", Port: 1, From: "sender@example.com", BaseURL: "https://example.com", TLSMode: "starttls"}
	if err := settings.NewService(db).UpdateSMTP(context.Background(), smtp); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM settings WHERE name = ?", "smtp")
	router := NewRouter(setup.NewService(store, setupBackend{}), provider)
	for _, email := range []string{user.Email, "missing@example.com"} {
		body, _ := json.Marshal(map[string]string{"email": email})
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset-requests", bytes.NewReader(body)))
		if response.Code != http.StatusAccepted {
			t.Fatalf("reset request returned %d", response.Code)
		}
	}
}

func TestWebSessionSurvivesRepeatedRequestsAndLogout(t *testing.T) {
	driver := os.Getenv("LIBREFANZA_TEST_DB_DRIVER")
	if driver == "" {
		t.Skip("需要设置测试数据库连接")
	}
	port, _ := strconv.Atoi(os.Getenv("LIBREFANZA_TEST_DB_PORT"))
	store := config.NewStore(t.TempDir())
	_ = store.Save(config.Config{Installed: true, Database: config.Database{Driver: driver, Host: "127.0.0.1", Port: port, Name: "librefanza_test", User: "test", Password: "integration-test-only"}})
	provider := database.NewProvider(store)
	defer provider.Close()
	db, err := provider.Database(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.NewService(db).Register(context.Background(), fmt.Sprintf("web-api-%d@example.com", time.Now().UnixNano()), "password123")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Delete(&user)
	service := setup.NewService(store, setupBackend{})
	defer service.Close()
	router := NewRouter(service, provider)
	body, _ := json.Marshal(map[string]string{"email": user.Email, "password": "password123", "client": "web"})
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions", bytes.NewReader(body)))
	if login.Code != http.StatusCreated || bytes.Contains(login.Body.Bytes(), []byte("refresh_token")) {
		t.Fatalf("login: %d %s", login.Code, login.Body.String())
	}
	if len(login.Result().Cookies()) != 0 {
		t.Fatal("login unexpectedly returned a cookie")
	}
	var payload struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+payload.Data.AccessToken)
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("request %d: %d", i, response.Code)
		}
	}
	logout := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/current", nil)
	req.Header.Set("Authorization", "Bearer "+payload.Data.AccessToken)
	router.ServeHTTP(logout, req)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", logout.Code)
	}
	again := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+payload.Data.AccessToken)
	router.ServeHTTP(again, req)
	if again.Code != http.StatusUnauthorized {
		t.Fatalf("authentication after logout: %d", again.Code)
	}
}
