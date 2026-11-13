package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/libredmm/librefanza/backend/internal/sources"
)

func TestPlaybackFetchErrors(t *testing.T) {
	for _, tc := range []struct {
		reason               string
		sourceStatus, status int
		expected             string
	}{
		{"challenge", 403, http.StatusBadGateway, "playback_verification_required"},
		{"http", 403, http.StatusBadGateway, "playback_source_unavailable"},
		{"http", 404, http.StatusOK, `"data":[]`},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondPlaybackFetchError(c, &sources.RequestError{Reason: tc.reason, Status: tc.sourceStatus})
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.expected) {
			t.Fatalf("状态 %d，响应 %s", w.Code, w.Body.String())
		}
	}
}
