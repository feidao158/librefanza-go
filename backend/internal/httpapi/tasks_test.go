package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/setup"
	"github.com/libredmm/librefanza/backend/internal/tasks"
)

type fakeTasks struct{}

type failingTasks struct {
	fakeTasks
	err error
}

func (f failingTasks) EnrichmentStatus(context.Context, string) (tasks.MetadataStatus, error) {
	return tasks.MetadataStatus{}, f.err
}

func (f failingTasks) Get(context.Context, string) (tasks.Status, error) {
	return tasks.Status{}, f.err
}

func (fakeTasks) Enqueue(context.Context, string, bool) (string, error) { return "job-id", nil }
func (fakeTasks) Get(context.Context, string) (tasks.Status, error) {
	return tasks.Status{ID: "job-id", Status: "pending"}, nil
}
func (fakeTasks) EnqueueMaintenance(context.Context, string) (string, error) { return "job-id", nil }

func TestSearchJobAPI(t *testing.T) {
	store := config.NewStore(t.TempDir())
	router := NewRouter(setup.NewService(store, setupBackend{}))
	RegisterTaskRoutes(router, nil, func(context.Context) (TaskService, error) { return fakeTasks{}, nil })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/search-jobs", bytes.NewBufferString(`{"id":"ABC-123"}`)))
	if response.Code != http.StatusAccepted || !bytes.Contains(response.Body.Bytes(), []byte("job-id")) {
		t.Fatalf("search: %d %s", response.Code, response.Body.String())
	}
	forced := httptest.NewRecorder()
	router.ServeHTTP(forced, httptest.NewRequest(http.MethodPost, "/api/v1/search-jobs", bytes.NewBufferString(`{"id":"ABC-123","force":true}`)))
	if forced.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous force: %d %s", forced.Code, forced.Body.String())
	}
	maintenance := httptest.NewRecorder()
	router.ServeHTTP(maintenance, httptest.NewRequest(http.MethodPost, "/api/v1/admin/maintenance-jobs", bytes.NewBufferString(`{"type":"rebuild_movies"}`)))
	if maintenance.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous maintenance: %d", maintenance.Code)
	}
}

func TestSearchJobLookupDistinguishesMissingFromDatabaseFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code int
	}{
		{"missing", tasks.ErrJobNotFound, http.StatusNotFound},
		{"storage failure", errors.New("storage unavailable"), http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := setup.NewService(config.NewStore(t.TempDir()), setupBackend{})
			defer service.Close()
			router := NewRouter(service)
			RegisterTaskRoutes(router, nil, func(context.Context) (TaskService, error) { return failingTasks{err: test.err}, nil })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/search-jobs/job-id", nil))
			if response.Code != test.code {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMetadataJobLookupDistinguishesMissingFromDatabaseFailure(t *testing.T) {
	for _, test := range []struct {
		err    error
		status int
	}{{tasks.ErrJobNotFound, http.StatusNotFound}, {errors.New("database unavailable"), http.StatusServiceUnavailable}} {
		service := setup.NewService(config.NewStore(t.TempDir()), setupBackend{})
		router := NewRouter(service)
		RegisterTaskRoutes(router, nil, func(context.Context) (TaskService, error) { return failingTasks{err: test.err}, nil })
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/movies/ABC-123/metadata-job", nil))
		service.Close()
		if response.Code != test.status {
			t.Fatalf("metadata status = %d", response.Code)
		}
	}
}
