package runtime

import (
	"context"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/config"
	"github.com/libredmm/librefanza/backend/internal/database"
)

func TestRunnerDoesNotStartBeforeInstallation(t *testing.T) {
	provider := database.NewProvider(config.NewStore(t.TempDir()))
	runner := NewRunner(context.Background(), provider)
	defer runner.Close()
	if _, err := runner.Manager(context.Background()); err == nil {
		t.Fatal("started worker before installation")
	}
}
