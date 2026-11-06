package sources

import (
	"context"
	"errors"
	"testing"

	"github.com/libredmm/librefanza/backend/internal/catalog"
)

type stubProvider struct {
	name  string
	calls *[]string
	items []Parsed
	err   error
}

func (p stubProvider) Search(context.Context, string) ([]Parsed, error) {
	*p.calls = append(*p.calls, p.name)
	return p.items, p.err
}

func (p stubProvider) SourceName() string { return p.name }

type stubSink struct{ saved []Parsed }

func (s *stubSink) Save(_ context.Context, item Parsed) error {
	s.saved = append(s.saved, item)
	return nil
}

func TestSearchUsesSourceOrderAndStopsAtMatchingItem(t *testing.T) {
	var calls []string
	sink := &stubSink{}
	providers := []Provider{
		stubProvider{name: "fanza", calls: &calls},
		stubProvider{name: "sod", calls: &calls, items: []Parsed{{Item: catalog.SourceItem{NormalizedID: "ABC-123"}}}},
		stubProvider{name: "mgstage", calls: &calls},
	}
	found, err := Search(context.Background(), "abc123", providers, sink)
	if err != nil || !found || len(calls) != 2 || calls[0] != "fanza" || calls[1] != "sod" || len(sink.saved) != 1 {
		t.Fatalf("found=%v calls=%v saved=%v err=%v", found, calls, sink.saved, err)
	}
}

func TestSearchReportsSourceFailureWhenNoMatch(t *testing.T) {
	var calls []string
	providers := []Provider{stubProvider{name: "fanza", calls: &calls, err: errors.New("offline")}, stubProvider{name: "sod", calls: &calls}}
	if found, err := Search(context.Background(), "abc123", providers, &stubSink{}); found || err == nil || len(calls) != 2 {
		t.Fatalf("found=%v calls=%v err=%v", found, calls, err)
	}
}

func TestSearchReportsIndividualFailureEvenWhenAnotherSourceSucceeds(t *testing.T) {
	var calls []string
	cause := errors.New("offline")
	providers := []Provider{
		stubProvider{name: "fanza", calls: &calls, err: cause},
		stubProvider{name: "sod", calls: &calls, items: []Parsed{{Item: catalog.SourceItem{NormalizedID: "ABC-123"}}}},
	}
	var failed string
	found, err := Search(context.Background(), "ABC-123", providers, &stubSink{}, func(source string, err error) {
		if errors.Is(err, cause) {
			failed = source
		}
	})
	if err != nil || !found || failed != "fanza" {
		t.Fatalf("检索结果 = %v，失败来源 = %q，错误 = %v", found, failed, err)
	}
}
