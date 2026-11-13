package torrentplay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestExpectedIncompleteChunkDoesNotFloodLogs(t *testing.T) {
	var out bytes.Buffer
	h := quietTransientReader{slog.NewTextHandler(&out, nil)}
	logger := slog.New(h)
	logger.ErrorContext(context.Background(), "initial read failed", "err", io.EOF)
	logger.ErrorContext(context.Background(), "initial read failed", "err", errors.New("disk failed"))
	if strings.Count(out.String(), "initial read failed") != 1 || !strings.Contains(out.String(), "disk failed") {
		t.Fatalf("日志过滤错误：%s", out.String())
	}
}
