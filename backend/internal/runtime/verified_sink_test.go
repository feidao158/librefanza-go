package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/sources"
)

type testCrawlSink struct{ err error }

func (s *testCrawlSink) Save(context.Context, sources.Parsed) error { return s.err }

func TestVerifiedCrawlSink(t *testing.T) {
	ctx := context.Background()
	saveErr := errors.New("保存失败")
	markErr := errors.New("标记失败")
	base := &testCrawlSink{err: saveErr}
	marks := 0
	mark := func(context.Context, string) error { marks++; return markErr }
	sink := &verifiedCrawlSink{sink: base, source: "javbus", mark: mark}
	if err := sink.Save(ctx, sources.Parsed{}); !errors.Is(err, saveErr) || marks != 0 {
		t.Fatalf("保存失败时不应标记: %v, %d", err, marks)
	}
	base.err = nil
	if err := sink.Save(ctx, sources.Parsed{}); !errors.Is(err, markErr) || marks != 1 {
		t.Fatalf("标记失败必须返回错误: %v, %d", err, marks)
	}
	markErr = nil
	if err := sink.Save(ctx, sources.Parsed{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.Save(ctx, sources.Parsed{}); err != nil || marks != 2 {
		t.Fatalf("成功后不应重复标记: %v, %d", err, marks)
	}
}
