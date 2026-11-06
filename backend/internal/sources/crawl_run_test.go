package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/libredmm/librefanza/backend/internal/catalog"
)

type crawlStub struct {
	failPage int
	pages    []int
}

func TestAVMOOPageLoadsIndividualDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/jav/data/api/getMovies":
			fmt.Fprint(w, `{"code":200,"data":[{"movieId":"abc123","movieFanHao":"ABC-123"}]}`)
		case "/jav/data/api/getMovie":
			fmt.Fprint(w, `{"code":200,"data":{"movieId":"abc123","movieFanHao":"ABC-123","title":"AVMOO title"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	provider := &httpProvider{kind: "avmoo", base: base, client: server.Client()}
	items, err := provider.Page(context.Background(), 1)
	if err != nil || len(items) != 1 || items[0].Item.NormalizedID != "ABC-123" {
		t.Fatalf("采集结果 = %+v，错误 = %v", items, err)
	}
}

func (s *crawlStub) Page(_ context.Context, page int) ([]Parsed, error) {
	s.pages = append(s.pages, page)
	if page == s.failPage {
		return nil, errors.New("连接失败")
	}
	if page > 3 {
		return nil, nil
	}
	date := time.Date(2025, 1, page, 0, 0, 0, 0, time.UTC)
	return []Parsed{{Item: catalog.SourceItem{Source: "avmoo", ExternalID: fmt.Sprint(page), NormalizedID: fmt.Sprintf("ABC-%03d", page), ReleaseDate: &date}}}, nil
}

func (s *crawlStub) Search(_ context.Context, id string) ([]Parsed, error) {
	return []Parsed{{Item: catalog.SourceItem{Source: "avmoo", ExternalID: id, NormalizedID: id}}}, nil
}

func TestCrawlResumeAndPageRange(t *testing.T) {
	request := CrawlRequest{Source: "avmoo", Mode: "pages", StartPage: 1, EndPage: 3}
	progress := CrawlProgress{}
	source := &crawlStub{failPage: 3}
	sink := &stubSink{}
	save := func(value CrawlProgress) error { progress = value; return nil }
	if err := RunCrawl(context.Background(), request, source, sink, progress, save); err == nil || progress.NextPage != 3 {
		t.Fatalf("进度 = %+v", progress)
	}
	source.failPage = 0
	source.pages = nil
	if err := RunCrawl(context.Background(), request, source, sink, progress, save); err != nil {
		t.Fatal(err)
	}
	if len(source.pages) != 2 || source.pages[0] != 2 || source.pages[1] != 3 || progress.NextPage != 4 {
		t.Fatalf("重扫页 = %v，进度 = %+v", source.pages, progress)
	}
}

func TestCrawlDateAndIDRanges(t *testing.T) {
	source := &crawlStub{}
	sink := &stubSink{}
	var progress CrawlProgress
	save := func(value CrawlProgress) error { progress = value; return nil }
	date := CrawlRequest{Source: "avmoo", Mode: "dates", StartDate: "2025-01-02", EndDate: "2025-01-03"}
	if err := RunCrawl(context.Background(), date, source, sink, progress, save); err != nil || len(sink.saved) != 2 {
		t.Fatalf("日期采集数量 = %d，错误 = %v", len(sink.saved), err)
	}
	sink.saved = nil
	progress = CrawlProgress{}
	ids := CrawlRequest{Source: "avmoo", Mode: "ids", Prefix: "ABC", StartNumber: 1, EndNumber: 2, NumberWidth: 3}
	if err := RunCrawl(context.Background(), ids, source, sink, progress, save); err != nil || len(sink.saved) != 2 || sink.saved[1].Item.NormalizedID != "ABC-002" {
		t.Fatalf("番号采集结果 = %+v，错误 = %v", sink.saved, err)
	}
}

func TestCrawlRequestRejectsInvalidRanges(t *testing.T) {
	for _, request := range []CrawlRequest{
		{Source: "unknown", Mode: "full"},
		{Source: "avmoo", Mode: "pages", StartPage: 3, EndPage: 2},
		{Source: "avmoo", Mode: "dates", StartDate: "2025-02-30", EndDate: "2025-03-01"},
		{Source: "jable", Mode: "dates", StartDate: "2025-01-01", EndDate: "2025-01-02"},
		{Source: "avmoo", Mode: "ids", Prefix: "ABC/", StartNumber: 1, EndNumber: 2},
	} {
		if err := request.Validate(); err == nil {
			t.Errorf("无效请求通过校验：%+v", request)
		}
	}
}
