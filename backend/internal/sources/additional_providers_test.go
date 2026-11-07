package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestAdditionalProvidersSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movies":
			fmt.Fprint(w, `<a href="/movies/ABC-123">ABC-123</a>`)
		case "/movies/ABC-123.json":
			fmt.Fprint(w, `{"normalized_id":"ABC-123","title":"Libre title","date":"2025-01-02","volume":7200,"actresses":[{"name":"演员甲"}]}`)
		case "/jav/data/api/search":
			if r.Method != http.MethodPost {
				t.Errorf("AVMOO 搜索请求方法 = %s", r.Method)
			}
			fmt.Fprint(w, `{"code":200,"data":[{"movieId":"movie123","movieFanHao":"ABC-123"}]}`)
		case "/jav/data/api/getMovie":
			fmt.Fprint(w, `{"code":200,"data":{"movieId":"movie123","movieFanHao":"ABC-123","title":"AVMOO title","releaseDate":"2025-01-02","length":120,"star":[{"starName":"演员甲"}]}}`)
		case "/ja/vl_searchbyid.php":
			fmt.Fprint(w, `<div class="videos"><div class="video"><a href="?v=abc123">ABC-123</a></div></div>`)
		case "/ja/":
			fmt.Fprint(w, `<div id="video_id"><table><tr><td class="text">ABC-123</td></tr></table></div><div id="video_title"><h3>ABC-123 Library title</h3></div>`)
		case "/videos/abc-123/":
			fmt.Fprint(w, `<html><head><meta property="og:title" content="ABC-123 Jable title"><meta property="og:image" content="https://example.com/cover.jpg"></head><body><h4>ABC-123 Jable title</h4></body></html>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"libredmm", "jable"} {
		t.Run(name, func(t *testing.T) {
			provider := &httpProvider{kind: name, base: base, client: server.Client()}
			items, err := provider.Search(context.Background(), "ABC-123")
			if err != nil || len(items) != 1 || items[0].Item.Source != name || items[0].Item.NormalizedID != "ABC-123" || items[0].Item.Title == "" {
				t.Fatalf("结果 = %+v，错误 = %v", items, err)
			}
		})
	}
}

func TestAdditionalProviderRejectsChallengePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><form id="challenge-form"></form></html>`)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL + "/")
	provider := &httpProvider{kind: "jable", base: base, client: server.Client()}
	if _, err := provider.Search(context.Background(), "ABC-123"); err == nil {
		t.Fatal("站点验证页不应当被当作没有结果")
	}
}

func TestNewProvidersRegistersAdditionalSources(t *testing.T) {
	providers, err := NewProviders(ClientSettings{})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, entry := range providers {
		seen[entry.(*httpProvider).kind] = true
	}
	for _, name := range []string{"jable", "libredmm", "javdb", "javbooks", "javquery"} {
		if !seen[name] {
			t.Errorf("缺少采集源 %s", name)
		}
	}
}

func TestActiveSourcesExcludeRetiredProviders(t *testing.T) {
	providers, err := NewProviders(ClientSettings{})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, provider := range providers {
		seen[provider.(*httpProvider).kind] = true
	}
	for _, name := range []string{"javdb", "javbooks", "javquery"} {
		if !seen[name] {
			t.Errorf("缺少采集源 %s", name)
		}
	}
	for _, name := range []string{"avmoo", "javlibrary", "javbus"} {
		if seen[name] {
			t.Errorf("已退役来源仍会被采集: %s", name)
		}
		if err := (CrawlRequest{Source: name, Mode: "incremental"}).Validate(); err == nil {
			t.Errorf("已退役来源仍接受新任务: %s", name)
		}
	}
}

func TestAdditionalSourceBaseURLsRejectCredentials(t *testing.T) {
	for _, value := range []string{"https://user:password@example.com/", "https://example.com/?token=secret", "https://example.com/#secret"} {
		if _, err := NewProviders(ClientSettings{JableURL: value}); err == nil {
			t.Fatal("来源地址不应包含凭据或查询参数")
		}
	}
}

func TestJableUsesConfiguredSessionCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=example" {
			t.Error("未发送来源会话")
		}
		fmt.Fprint(w, `<h4>ABC-123 title</h4>`)
	}))
	defer server.Close()
	providers, err := NewProviders(ClientSettings{JableURL: server.URL, JableCookie: "session=example"})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range providers {
		if entry.(*httpProvider).kind == "jable" {
			if _, err := entry.Search(context.Background(), "ABC-123"); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("未注册 Jable 来源")
}

func TestLiveAdditionalSource(t *testing.T) {
	source := os.Getenv("LIBREFANZA_LIVE_SOURCE")
	if source == "" {
		t.Skip("需要显式指定在线采集源")
	}
	ids := map[string]string{"libredmm": "PPPE-460", "jable": "SORA-655", "javdb": "ABP-123", "javbus": "ABP-123", "javbooks": "LUCY-032", "javquery": "SDDE-772"}
	id, ok := ids[source]
	if !ok {
		t.Fatal("无效的在线采集源")
	}
	provider, err := NewCrawlSource(ClientSettings{}, source)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	items, err := provider.Search(ctx, id)
	if err != nil || len(items) == 0 || items[0].Item.Title == "" {
		t.Fatalf("来源 %s 无法检索：%v", source, err)
	}
}

func TestReparseAdditionalJSONSources(t *testing.T) {
	for _, test := range []struct{ source, address, body string }{
		{"avmoo", "https://avmoo.shop/ja/movies/item123", `{"movieId":"item123","movieFanHao":"ABC-123","title":"标题"}`},
		{"libredmm", "https://www.libredmm.com/movies/ABC-123", `{"normalized_id":"ABC-123","title":"标题"}`},
	} {
		item, err := ParseRemoteJSON(test.source, test.address, []byte(test.body))
		if err != nil || item.Item.NormalizedID != "ABC-123" || item.Item.Source != test.source {
			t.Fatalf("来源 = %s，结果 = %+v，错误 = %v", test.source, item, err)
		}
	}
}
