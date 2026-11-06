package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
	"github.com/libredmm/librefanza/backend/internal/setup"
)

type setupBackend struct{}

func (setupBackend) CheckDatabase(context.Context, config.Database) error { return nil }
func (setupBackend) CheckRedis(context.Context, setup.Redis) error        { return nil }
func (setupBackend) Prepare(context.Context, config.Database, setup.Redis, string, string) error {
	return nil
}

func TestSetupAPIClosesAfterInitialization(t *testing.T) {
	service := setup.NewService(config.NewStore(t.TempDir()), setupBackend{})
	router := NewRouter(service)
	status := httptest.NewRecorder()
	router.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/v1/setup", nil))
	if status.Code != http.StatusOK || !bytes.Contains(status.Body.Bytes(), []byte(`"installed":false`)) {
		t.Fatalf("initial status: %d %s", status.Code, status.Body.String())
	}
	request := setup.Request{Database: config.Database{Driver: "postgres", Host: "localhost", Port: 5432, Name: "app", User: "app", Password: "private"}, Email: "admin@example.com", Password: "password123"}
	body, _ := json.Marshal(request)
	install := httptest.NewRecorder()
	router.ServeHTTP(install, httptest.NewRequest(http.MethodPost, "/api/v1/setup/installations", bytes.NewReader(body)))
	if install.Code != http.StatusCreated || bytes.Contains(install.Body.Bytes(), []byte("private")) {
		t.Fatalf("install: %d %s", install.Code, install.Body.String())
	}
	repeat := httptest.NewRecorder()
	router.ServeHTTP(repeat, httptest.NewRequest(http.MethodPost, "/api/v1/setup/installations", bytes.NewReader(body)))
	if repeat.Code != http.StatusConflict {
		t.Fatalf("repeat install: %d %s", repeat.Code, repeat.Body.String())
	}
}

func TestOpenAPIDocumentIsAvailable(t *testing.T) {
	service := setup.NewService(config.NewStore(t.TempDir()), setupBackend{})
	defer service.Close()
	router := NewRouter(service)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("openapi:")) || !bytes.Contains(response.Body.Bytes(), []byte("/movies")) {
		t.Fatalf("OpenAPI: %d %s", response.Code, response.Body.String())
	}
	document, err := openapi3.NewLoader().LoadFromData(response.Body.Bytes())
	if err != nil {
		t.Fatalf("invalid OpenAPI document: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("invalid OpenAPI contract: %v", err)
	}
}

func TestOpenAPIDocumentsEveryAPIRoute(t *testing.T) {
	store := config.NewStore(t.TempDir())
	service := setup.NewService(store, setupBackend{})
	defer service.Close()
	provider := database.NewProvider(store)
	router := NewRouter(service, provider)
	RegisterTaskRoutes(router, provider, func(context.Context) (TaskService, error) { return fakeTasks{}, nil })
	doc, err := openapi3.NewLoader().LoadFromData(openAPIDocument)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range router.Routes() {
		path := strings.TrimPrefix(route.Path, "/api/v1")
		path = strings.ReplaceAll(path, ":id", "{id}")
		path = strings.ReplaceAll(path, ":variant", "{variant}")
		path = strings.ReplaceAll(path, ":token", "{token}")
		item := doc.Paths.Find(path)
		if item == nil || item.GetOperation(route.Method) == nil {
			t.Errorf("undocumented route %s %s", route.Method, route.Path)
		}
	}
}

func TestSetupRequestsAreRateLimited(t *testing.T) {
	service := setup.NewService(config.NewStore(t.TempDir()), setupBackend{})
	defer service.Close()
	router := NewRouter(service)
	var last *httptest.ResponseRecorder
	for i := 0; i < 15; i++ {
		last = httptest.NewRecorder()
		router.ServeHTTP(last, httptest.NewRequest(http.MethodPost, "/api/v1/setup/installations", bytes.NewBufferString(`{}`)))
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("last status = %d", last.Code)
	}
}
