package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewSourceDetailParsers(t *testing.T) {
	tests := []struct {
		source, address, body string
	}{
		{"javdb", "https://javdb.com/v/abc", `<h2 class="title"><strong>ABC-123</strong><strong class="current-title">标题</strong></h2><img class="video-cover" src="https://images.example/large.jpg"><div class="panel-block"><strong>ID:</strong><span class="value">ABC-123</span></div><div class="panel-block"><strong>Released Date:</strong><span class="value">2025-01-02</span></div><div class="panel-block"><strong>Actor(s):</strong><span class="value"><a>演员甲</a></span></div><div class="panel-block"><strong>Tags:</strong><span class="value"><a>类别甲</a></span></div><div class="preview-images"><a data-fancybox="gallery" href="https://images.example/sample.jpg">图</a></div>`},
		{"javbooks", "https://javbooks.com/content_censored/1.htm", `<div id="title"><b>标题</b></div><div id="info"><div class="info_cg"><img src="https://images.example/large.jpg"></div><div class="infobox"><b>番號：</b><a>ABC-123</a></div><div class="infobox"><b>發行時間：</b>2025-01-02</div><div class="infobox"><b>女優：</b><a>演员甲</a></div><div class="infobox"><b>影片類別：</b><a>类别甲</a></div></div><div class="gallery"><a href="https://images.example/sample.jpg"><img src="https://images.example/small.jpg"></a></div>`},
		{"javquery", "https://javquery.cc/post/1/abc-123.html", `<main class="ym-main"><article class="ym-detail__main"><h1 class="ym-title">标题</h1><p class="ym-detail__meta"><span class="ym-code">ABC-123</span><time datetime="2025-01-02"></time></p><div class="ym-detail__cover"><img src="https://images.example/large.jpg"></div><dl class="ym-info"><dt>演员</dt><dd><a>演员甲</a></dd><dt>类型</dt><dd><a>类别甲</a></dd></dl><div class="ym-detail__gallery"><template data-gallery-template><a href="https://images.example/sample.jpg">图</a></template></div></article></main>`},
		{"javbus", "https://www.javbus.com/ABC-123", `<h3>ABC-123 标题</h3><a class="bigImage" href="https://images.example/large.jpg"><img src="https://images.example/small.jpg"></a><div class="info"><p><span class="header">識別碼:</span> ABC-123</p><p><span class="header">發行日期:</span> 2025-01-02</p><p><span class="header">類別:</span><a>类别甲</a></p><a class="star-name">演员甲</a></div><a class="sample-box" href="https://images.example/sample.jpg">图</a>`},
	}
	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			item, err := ParsePage(tc.source, tc.address, []byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			var metadata Metadata
			if err := json.Unmarshal([]byte(item.Item.MetadataJSON), &metadata); err != nil {
				t.Fatal(err)
			}
			if item.Item.NormalizedID != "ABC-123" || item.Item.Title == "" || item.Item.ReleaseDate == nil || item.Item.CoverImageURL != "https://images.example/large.jpg" || len(metadata.Cast) != 1 || len(metadata.Genres) != 1 || len(metadata.Samples) != 1 || metadata.Samples[0] != "https://images.example/sample.jpg" {
				t.Fatalf("解析结果不完整: %+v %+v", item.Item, metadata)
			}
		})
	}
}

func TestJavDBPreviewDoesNotCaptureLoginOrRecommendations(t *testing.T) {
	body := `<h2 class="title"><strong class="current-title">标题</strong></h2><div class="panel-block"><strong>ID:</strong><span class="value">ABC-123</span></div><div class="preview-images"><a class="preview-video-container" href="/login">预告片</a><a data-fancybox="gallery" href="https://images.example/sample.jpg">预览</a></div><div id="gallery"><a class="tile-item" href="/v/related">其他影片</a></div>`
	item, err := ParsePage("javdb", "https://javdb.com/v/abc", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	var metadata Metadata
	if err := json.Unmarshal([]byte(item.Item.MetadataJSON), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.Samples) != 1 || metadata.Samples[0] != "https://images.example/sample.jpg" {
		t.Fatalf("错误捕获非图片链接: %v", metadata.Samples)
	}
}

func TestNewSourcesDistinguishEmptyResultsFromUnknownPages(t *testing.T) {
	for _, tc := range []struct {
		source, empty string
	}{
		{"javbooks", `<div>搜尋到0筆資料。</div>`},
		{"javquery", `<main><div class="ym-search-results">没有搜索结果</div></main>`},
	} {
		t.Run(tc.source, func(t *testing.T) {
			base, _ := url.Parse("https://example.com/")
			provider := &httpProvider{kind: tc.source, base: base}
			links, err := provider.newSourceLinks(base, []byte(tc.empty))
			if err != nil || len(links) != 0 {
				t.Fatalf("明确的空结果应被识别：%v %v", links, err)
			}
			if _, err := provider.newSourceLinks(base, []byte(`<div id="main">网页结构变化</div>`)); err == nil {
				t.Fatal("普通页面不能被当作空搜索结果")
			}
		})
	}
}

func TestJavQueryIncludesFC2DetailLinks(t *testing.T) {
	base, _ := url.Parse("https://javquery.cc/")
	provider := &httpProvider{kind: "javquery", base: base}
	links, err := provider.newSourceLinks(base, []byte(`<div class="ym-search-results"><article class="ym-card"><a href="/fc2-post/123/fc2-123.html">FC2-123</a></article></div>`))
	if err != nil || len(links) != 1 {
		t.Fatalf("未识别 FC2 详情地址: %v %v", links, err)
	}
}

func TestSourceDetailDistinguishesDeletedAndUnexpectedPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `<html><body>页面结构已变化</body></html>`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	provider := &httpProvider{kind: "javdb", base: base, client: server.Client()}
	if _, err := provider.newSourceDetail(context.Background(), server.URL+"/missing"); !errors.Is(err, errInvalidProduct) {
		t.Fatalf("404 应当作为失效条目跳过: %v", err)
	}
	if _, err := provider.newSourceDetail(context.Background(), server.URL+"/unexpected"); err == nil || errors.Is(err, errInvalidProduct) {
		t.Fatalf("未知结构不应作为单个失效条目跳过: %v", err)
	}
}

func TestRenderedDetailTreats404AsDeleted(t *testing.T) {
	base, _ := url.Parse("https://javquery.cc/")
	provider := &httpProvider{kind: "javquery", base: base, browserFetch: func(context.Context, string, string) ([]byte, *url.URL, error) {
		return nil, base, &RequestError{Reason: "http", Status: http.StatusNotFound}
	}}
	if _, err := provider.newSourceDetail(context.Background(), "https://javquery.cc/post/1/missing.html"); !errors.Is(err, errInvalidProduct) {
		t.Fatalf("浏览器访问的失效详情未跳过: %v", err)
	}
}

func TestJavBooksNetworkFailureFallsBackToBrowser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/serch_censored.htm":
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		case "/content_censored/1.htm":
			fmt.Fprint(w, `<div id="title">标题</div><div id="info"><div class="infobox"><b>番號：</b><a>ABC-123</a></div></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	called := false
	provider := &httpProvider{kind: "javbooks", base: base, client: server.Client(), browserFetch: func(_ context.Context, _ string, search string) ([]byte, *url.URL, error) {
		called = search == "ABC-123"
		return []byte(`<a href="/content_censored/1.htm">ABC-123</a>`), base, nil
	}}
	items, err := provider.Search(context.Background(), "ABC-123")
	if err != nil || !called || len(items) != 1 || items[0].Item.NormalizedID != "ABC-123" {
		t.Fatalf("浏览器回退失败: 调用=%v 结果=%+v 错误=%v", called, items, err)
	}
}

func TestJavBooksForbiddenDoesNotTriggerBrowserFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	called := false
	provider := &httpProvider{kind: "javbooks", base: base, client: server.Client(), browserFetch: func(context.Context, string, string) ([]byte, *url.URL, error) {
		called = true
		return nil, base, nil
	}}
	_, err := provider.Search(context.Background(), "ABC-123")
	var requestErr *RequestError
	if called || !errors.As(err, &requestErr) || requestErr.Status != http.StatusForbidden {
		t.Fatalf("验证或拒绝不应触发浏览器回退: 调用=%v 错误=%v", called, err)
	}
}

func TestSourceListRejectsTooManyLinks(t *testing.T) {
	base, _ := url.Parse("https://javdb.com/")
	provider := &httpProvider{kind: "javdb", base: base}
	var body strings.Builder
	body.WriteString(`<div class="movie-list">`)
	for i := 0; i < 101; i++ {
		fmt.Fprintf(&body, `<a href="/v/%d">作品</a>`, i)
	}
	body.WriteString(`</div>`)
	if _, err := provider.newSourceLinks(base, []byte(body.String())); err == nil {
		t.Fatal("不能静默截断超限列表")
	}
}

type pausedHealth struct{}

func (pausedHealth) Begin(context.Context, string) (time.Time, error) {
	return time.Now().Add(time.Hour), nil
}
func (pausedHealth) End(context.Context, string, error) (bool, error) { return false, nil }

type countingHealth struct{ begins, ends int }

func (h *countingHealth) Begin(context.Context, string) (time.Time, error) {
	h.begins++
	return time.Time{}, nil
}
func (h *countingHealth) End(context.Context, string, error) (bool, error) {
	h.ends++
	return true, nil
}

func TestInapplicableProductDoesNotChangeSourceHealth(t *testing.T) {
	health := &countingHealth{}
	provider := &httpProvider{kind: "fc2", settings: ClientSettings{Health: health}}
	items, err := provider.Search(context.Background(), "ABC-123")
	if err != nil || len(items) != 0 || health.begins != 0 || health.ends != 0 {
		t.Fatalf("不适用番号不应改变来源状态: %v %+v", err, health)
	}
}

func TestMissingFanzaCredentialsAreNotReportedAsEmptyResult(t *testing.T) {
	provider := &httpProvider{kind: "fanza"}
	if _, err := provider.Search(context.Background(), "ABC-123"); err == nil || !strings.Contains(err.Error(), "credentials are not configured") {
		t.Fatalf("缺少凭据不应视为无结果: %v", err)
	}
}

func TestScheduledSourceTaskHonorsCooldown(t *testing.T) {
	called := false
	err := RunSourceTask(context.Background(), ClientSettings{Health: pausedHealth{}}, "mgstage", func() error {
		called = true
		return nil
	})
	var deferred *DeferredError
	if !errors.As(err, &deferred) || called {
		t.Fatalf("定时采集未遵守冷却: %v %v", called, err)
	}
}

func TestExistingSourcePageSkipsMissingDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movies":
			fmt.Fprint(w, `<div class="movies"><a href="/movies/ABC-123">一</a><a href="/movies/ABC-124">二</a></div>`)
		case "/movies/ABC-123.json":
			http.NotFound(w, r)
		case "/movies/ABC-124.json":
			fmt.Fprint(w, `{"normalized_id":"ABC-124","title":"标题"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	provider := &httpProvider{kind: "libredmm", base: base, client: server.Client()}
	items, skipped, err := provider.PageWithStats(context.Background(), 1)
	if err != nil || skipped != 1 || len(items) != 1 || items[0].Item.NormalizedID != "ABC-124" {
		t.Fatalf("旧来源未跳过失效详情: %+v %d %v", items, skipped, err)
	}
}

func TestSourceImageURLRejectsCredentialsAndEmptyValues(t *testing.T) {
	for _, value := range []string{"", "https://user:password@images.example/cover.jpg"} {
		body := `<h2 class="title"><strong class="current-title">标题</strong></h2><img class="video-cover" src="` + value + `"><div class="panel-block"><strong>ID:</strong><span class="value">ABC-123</span></div>`
		item, err := ParsePage("javdb", "https://javdb.com/v/abc", []byte(body))
		if err != nil || item.Item.CoverImageURL != "" {
			t.Fatalf("不应保存无效图片地址: %q %v", item.Item.CoverImageURL, err)
		}
	}
}

type verifiedHealth struct{ verified int }

func (h *verifiedHealth) Begin(context.Context, string) (time.Time, error) { return time.Time{}, nil }
func (h *verifiedHealth) End(context.Context, string, error) (bool, error) { return true, nil }
func (h *verifiedHealth) MarkVerified(context.Context, string) error       { h.verified++; return nil }

func TestSourcePageDoesNotMarkVerificationBeforeSave(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<div class="movie-list"><a href="/v/abc">ABC-123</a></div>`)
			return
		}
		fmt.Fprint(w, `<h2 class="title"><strong class="current-title">标题</strong></h2><div class="panel-block"><strong>ID:</strong><span class="value">ABC-123</span></div>`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	health := &verifiedHealth{}
	provider := &httpProvider{kind: "javdb", base: base, client: server.Client(), settings: ClientSettings{Health: health}}
	items, _, err := provider.PageWithStats(context.Background(), 1)
	if err != nil || len(items) != 1 || health.verified != 0 {
		t.Fatalf("尚未入库的来源不应标记为已验证: %v %d %v", items, health.verified, err)
	}
}

func TestProbeSourceLimitsDetailRequests(t *testing.T) {
	details := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fmt.Fprint(w, `<div class="movie-list"><a href="/v/1">1</a><a href="/v/2">2</a><a href="/v/3">3</a><a href="/v/4">4</a></div>`)
			return
		}
		details++
		http.NotFound(w, r)
	}))
	defer server.Close()
	_, err := ProbeSource(context.Background(), ClientSettings{JavDBURL: server.URL}, "javdb")
	if err == nil || details != 3 {
		t.Fatalf("应在三个无效详情后停止: 次数=%d 错误=%v", details, err)
	}
}

func TestUnavailableSourceIsDeferredWithoutNetworkRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	providers, err := NewProviders(ClientSettings{JavDBURL: server.URL, Health: pausedHealth{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range providers {
		if provider.(*httpProvider).kind == "javdb" {
			_, err := provider.Search(context.Background(), "ABC-123")
			var deferred *DeferredError
			if !errors.As(err, &deferred) || called {
				t.Fatalf("来源未延期或发生了请求: %v %v", err, called)
			}
			return
		}
	}
	t.Fatal("缺少 JavDB 来源")
}

func TestJavDBSearchFetchesMatchingDetailAndRejectsUnknownPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			if r.URL.Query().Get("q") == "EMPTY-999" {
				fmt.Fprint(w, `<html><body>unexpected layout</body></html>`)
				return
			}
			fmt.Fprint(w, `<div class="movie-list"><div class="item"><a href="/v/abc"><div class="video-title"><strong>ABC-123</strong></div></a></div></div>`)
		case "/v/abc":
			fmt.Fprint(w, `<h2 class="title"><strong class="current-title">标题</strong></h2><div class="panel-block"><strong>ID:</strong><span class="value">ABC-123</span></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	providers, err := NewProviders(ClientSettings{JavDBURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range providers {
		if entry.(*httpProvider).kind != "javdb" {
			continue
		}
		items, err := entry.Search(context.Background(), "ABC-123")
		if err != nil || len(items) != 1 || items[0].Item.NormalizedID != "ABC-123" {
			t.Fatalf("检索失败: %+v %v", items, err)
		}
		if _, err := entry.Search(context.Background(), "EMPTY-999"); err == nil || !strings.Contains(err.Error(), "invalid source page") {
			t.Fatalf("未知页面不应视为空结果: %v", err)
		}
		return
	}
	t.Fatal("缺少 JavDB 来源")
}
