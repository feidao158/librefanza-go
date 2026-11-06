package sources

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPProviders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/ItemList":
			fmt.Fprint(w, `{"result":{"result_count":1,"total_count":1,"first_position":1,"items":[{"content_id":"abc00123","title":"Fanza item"}]}}`)
		case r.URL.Path == "/prime/_ontime.php":
			if r.Header.Get("Referer") == "" || !strings.Contains(r.Header.Get("Cookie"), "PHPSESSID=") {
				t.Error("missing SOD headers")
			}
			fmt.Fprint(w, `<div id="videos_head"><h1>SOD item</h1></div><table id="v_introduction"><tr><td class="v_intr_ti">品番</td><td class="v_intr_tx">ABC-123</td></tr></table>`)
		case r.URL.Path == "/search/cSearch.php":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprint(w, `<div class="search_list"><h5><a href="/product/abc">item</a></h5></div>`)
			}
		case r.URL.Path == "/product/abc":
			fmt.Fprint(w, `<h1 class="tag">MGS item</h1><div class="detail_data"><table><tr><th>品番</th><td>ABC-123</td></tr></table></div>`)
		case r.URL.Path == "/article/123/":
			fmt.Fprint(w, `<div class="items_article_headerInfo"><h3>FC2 item</h3></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	settings := ClientSettings{FanzaURL: server.URL + "/", FanzaAPIID: "api", FanzaAffiliateID: "affiliate", SODURL: server.URL + "/", MGStageURL: server.URL + "/", FC2URL: server.URL + "/"}
	providers, err := NewProviders(settings)
	if err != nil {
		t.Fatal(err)
	}
	for i, expected := range []string{"fanza", "sod", "mgstage"} {
		items, err := providers[i].Search(context.Background(), "ABC-123")
		if err != nil || len(items) == 0 || items[0].Item.Source != expected || items[0].Item.NormalizedID != "ABC-123" {
			t.Fatalf("%s: items=%v err=%v", expected, items, err)
		}
	}
	items, err := providers[3].Search(context.Background(), "FC2-123")
	if err != nil || len(items) != 1 || items[0].Item.NormalizedID != "FC2-123" {
		t.Fatalf("FC2: items=%v err=%v", items, err)
	}
}

func TestMGStageSearchUsesOriginalFixtures(t *testing.T) {
	searchPage, err := os.ReadFile(filepath.Join("testdata", "mgstage-search.html"))
	if err != nil {
		t.Fatal(err)
	}
	productPage, err := os.ReadFile(filepath.Join("testdata", "mgstage.html"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/cSearch.php":
			if r.URL.Query().Get("page") == "1" {
				_, _ = w.Write(searchPage)
			}
		case "/product/product_detail/261ARA-168/":
			_, _ = w.Write(productPage)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	providers, err := NewProviders(ClientSettings{MGStageURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	items, err := providers[2].Search(context.Background(), "ARA-168")
	if err != nil || len(items) != 1 || items[0].Item.NormalizedID != "ARA-168" || items[0].Item.Title == "" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestCrawlFanzaUsesDateRange(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("gte_date") == "" || r.URL.Query().Get("lte_date") == "" || r.URL.Query().Get("sort") != "date" {
			t.Errorf("missing crawl range: %s", r.URL.RawQuery)
		}
		requests++
		fmt.Fprint(w, `{"result":{"result_count":1,"total_count":1,"first_position":1,"items":[{"content_id":"abc00123","title":"Fanza item"}]}}`)
	}))
	defer server.Close()
	settings := ClientSettings{FanzaURL: server.URL + "/", FanzaAPIID: "api", FanzaAffiliateID: "affiliate"}
	sink := &stubSink{}
	if err := CrawlFanza(context.Background(), settings, 1, sink); err != nil || requests != 5 || len(sink.saved) != 1 {
		t.Fatalf("requests=%d saved=%d err=%v", requests, len(sink.saved), err)
	}
}

func TestCrawlActressesParsesAndTrimsRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ActressSearch" || r.URL.Query().Get("offset") != "1" {
			t.Errorf("unexpected request: %s", r.URL.String())
		}
		fmt.Fprint(w, `{"result":{"result_count":1,"total_count":1,"first_position":1,"actress":[{"id":"42","name":"　演员　","imageURL":{"large":"https://example.com/image.jpg"}}]}}`)
	}))
	defer server.Close()
	var records []ActressRecord
	err := CrawlActresses(context.Background(), ClientSettings{FanzaURL: server.URL, FanzaAPIID: "a", FanzaAffiliateID: "b"}, 1, func(record ActressRecord) error {
		records = append(records, record)
		return nil
	})
	if err != nil || len(records) != 1 || records[0].ID != 42 || records[0].Name != "演员" || records[0].ImageURL == "" {
		t.Fatalf("records=%+v err=%v", records, err)
	}
}

func TestCrawlMGStageReadsSeriesList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/series":
			fmt.Fprint(w, "abc")
		case "/search/cSearch.php":
			if r.URL.Query().Get("page") == "1" {
				fmt.Fprint(w, `<div class="search_list"><h5><a href="/product/abc">item</a></h5></div>`)
			}
		case "/product/abc":
			fmt.Fprint(w, `<h1 class="tag">MGS item</h1><div class="detail_data"><table><tr><th>品番</th><td>ABC-123</td></tr></table></div>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	sink := &stubSink{}
	err := CrawlMGStage(context.Background(), ClientSettings{MGStageURL: server.URL, MGStageSeriesURL: server.URL + "/series"}, sink)
	if err != nil || len(sink.saved) != 1 || sink.saved[0].Item.NormalizedID != "ABC-123" {
		t.Fatalf("saved=%+v err=%v", sink.saved, err)
	}
}

func TestFanzaSearchFetchesDescription(t *testing.T) {
	var base string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ItemList":
			fmt.Fprintf(w, `{"result":{"result_count":1,"total_count":1,"first_position":1,"items":[{"content_id":"abc123","title":"Example","URL":%q}]}}`, base+"/detail")
		case "/detail":
			if !strings.Contains(r.Header.Get("Cookie"), "age_check_done=1") {
				t.Error("missing age cookie")
			}
			fmt.Fprint(w, `<div class="mg-b20 lh4">　作品简介　</div>`)
		}
	}))
	defer server.Close()
	base = server.URL
	providers, err := NewProviders(ClientSettings{FanzaURL: base, FanzaAPIID: "a", FanzaAffiliateID: "b"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := providers[0].Search(context.Background(), "ABC-123")
	if err != nil || len(items) != 1 || items[0].Item.Description != "作品简介" {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}
