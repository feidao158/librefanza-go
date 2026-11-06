package tasks

import (
	"errors"
	"strings"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/logbook"
)

func TestTaskPanicIsSafelyRecordedAndReturnedForRetry(t *testing.T) {
	secret := "private-token-in-panic"
	_, err := protectTask(func() (bool, error) { panic(secret) })
	if !errors.Is(err, logbook.ErrTaskPanic) || strings.Contains(err.Error(), secret) {
		t.Fatalf("异常退出未安全转换为可重试错误：%v", err)
	}
	entry := logbook.Entry{}
	logbook.ApplyFailure(&entry, err)
	if entry.Reason != "panic" {
		t.Fatalf("异常退出原因 = %q", entry.Reason)
	}
}
