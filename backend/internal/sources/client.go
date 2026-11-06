package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"golang.org/x/time/rate"
)

type ClientSettings struct {
	FanzaURL              string `json:"fanza_url"`
	FanzaAPIID            string `json:"fanza_api_id"`
	FanzaAffiliateID      string `json:"fanza_affiliate_id"`
	SODURL                string `json:"sod_url"`
	MGStageURL            string `json:"mgstage_url"`
	MGStageSeriesURL      string `json:"mgstage_series_url"`
	FC2URL                string `json:"fc2_url"`
	JableURL              string `json:"jable_url"`
	AVMOOURL              string `json:"avmoo_url"`
	JavLibraryURL         string `json:"javlibrary_url"`
	LibreDMMURL           string `json:"libredmm_url"`
	JableCookie           string `json:"jable_cookie"`
	JavLibraryCookie      string `json:"javlibrary_cookie"`
	ClearJableCookie      bool   `json:"clear_jable_cookie,omitempty"`
	ClearJavLibraryCookie bool   `json:"clear_javlibrary_cookie,omitempty"`
	ProxyURL              string `json:"proxy_url"`
	ClearProxy            bool   `json:"clear_proxy,omitempty"`
}

type httpProvider struct {
	kind     string
	base     *url.URL
	client   *http.Client
	settings ClientSettings
	limiter  *rate.Limiter
}

func (p *httpProvider) SourceName() string { return p.kind }

func NewProviders(settings ClientSettings) ([]Provider, error) {
	for _, value := range []string{settings.JableCookie, settings.JavLibraryCookie} {
		if len(value) > 8192 || strings.ContainsAny(value, "\r\n") {
			return nil, errors.New("invalid source session")
		}
	}
	defaults := []struct{ kind, value, fallback string }{
		{"fanza", settings.FanzaURL, "https://api.dmm.com/affiliate/v3/"},
		{"sod", settings.SODURL, "https://ec.sod.co.jp/"},
		{"mgstage", settings.MGStageURL, "https://www.mgstage.com/"},
		{"fc2", settings.FC2URL, "https://adult.contents.fc2.com/"},
		{"javlibrary", settings.JavLibraryURL, "https://www.javlibrary.com/"},
		{"avmoo", settings.AVMOOURL, "https://avmoo.shop/"},
		{"jable", settings.JableURL, "https://jable.tv/"},
		{"libredmm", settings.LibreDMMURL, "https://www.libredmm.com/"},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if settings.ProxyURL != "" {
		proxy, err := url.Parse(settings.ProxyURL)
		if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Host == "" {
			return nil, errors.New("invalid proxy URL")
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return errors.New("redirect to another host is not allowed")
		}
		return nil
	}}
	providers := make([]Provider, 0, len(defaults))
	for _, entry := range defaults {
		value := entry.value
		if value == "" {
			value = entry.fallback
		}
		base, err := url.Parse(value)
		if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
			return nil, fmt.Errorf("invalid %s URL", entry.kind)
		}
		if (entry.kind == "jable" || entry.kind == "avmoo" || entry.kind == "javlibrary" || entry.kind == "libredmm") && (base.User != nil || base.RawQuery != "" || base.Fragment != "") {
			return nil, fmt.Errorf("invalid %s URL", entry.kind)
		}
		if !strings.HasSuffix(base.Path, "/") {
			base.Path += "/"
		}
		provider := &httpProvider{kind: entry.kind, base: base, client: client, settings: settings}
		if entry.kind == "jable" || entry.kind == "javlibrary" || entry.kind == "avmoo" || entry.kind == "libredmm" {
			provider.limiter = rate.NewLimiter(rate.Every(500*time.Millisecond), 1)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func (p *httpProvider) Search(ctx context.Context, id string) ([]Parsed, error) {
	if strings.HasPrefix(id, "FC2-") && (p.kind == "fanza" || p.kind == "sod" || p.kind == "mgstage") || !strings.HasPrefix(id, "FC2-") && p.kind == "fc2" {
		return nil, nil
	}
	switch p.kind {
	case "fanza":
		return p.searchFanza(ctx, id)
	case "sod":
		return p.searchSOD(ctx, id)
	case "mgstage":
		return p.searchMGStage(ctx, id)
	case "fc2":
		return p.searchFC2(ctx, id)
	case "libredmm":
		return p.searchLibreDMM(ctx, id)
	case "avmoo":
		return p.searchAVMOO(ctx, id)
	case "javlibrary":
		return p.searchJavLibrary(ctx, id)
	case "jable":
		return p.searchJable(ctx, id)
	default:
		return nil, errors.New("unsupported source")
	}
}

func (p *httpProvider) searchFanza(ctx context.Context, id string) ([]Parsed, error) {
	if p.settings.FanzaAPIID == "" || p.settings.FanzaAffiliateID == "" {
		return nil, nil
	}
	var result []Parsed
	seen := map[string]bool{}
	for _, variation := range catalog.ProductIDVariations(id) {
		err := p.fanzaItems(ctx, variation, "match", "", "", func(item Parsed) error {
			if !seen[item.Item.ExternalID] {
				seen[item.Item.ExternalID] = true
				p.enrichFanza(ctx, &item)
				result = append(result, item)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (p *httpProvider) fanzaItems(ctx context.Context, keyword, order, startDate, endDate string, consume func(Parsed) error) error {
	for _, floor := range [][2]string{{"mono", "dvd"}, {"digital", "nikkatsu"}, {"digital", "video"}, {"digital", "videoa"}, {"digital", "videoc"}} {
		for offset := 1; offset <= 100000; {
			endpoint := p.base.ResolveReference(&url.URL{Path: p.base.Path + "ItemList"})
			values := endpoint.Query()
			for key, value := range map[string]string{"api_id": p.settings.FanzaAPIID, "affiliate_id": p.settings.FanzaAffiliateID, "site": "FANZA", "service": floor[0], "floor": floor[1], "hits": "100", "offset": strconv.Itoa(offset), "sort": order, "output": "json"} {
				values.Set(key, value)
			}
			if keyword != "" {
				values.Set("keyword", keyword)
			}
			if startDate != "" {
				values.Set("gte_date", startDate)
			}
			if endDate != "" {
				values.Set("lte_date", endDate)
			}
			endpoint.RawQuery = values.Encode()
			body, _, err := p.get(ctx, endpoint.String(), nil)
			if err != nil {
				return err
			}
			var response struct {
				Result struct {
					Items         []json.RawMessage `json:"items"`
					ResultCount   json.RawMessage   `json:"result_count"`
					TotalCount    json.RawMessage   `json:"total_count"`
					FirstPosition json.RawMessage   `json:"first_position"`
				} `json:"result"`
			}
			if json.Unmarshal(body, &response) != nil {
				return errors.New("invalid Fanza response")
			}
			for _, raw := range response.Result.Items {
				item, err := ParseFanza(raw)
				if err == nil {
					if err := consume(item); err != nil {
						return err
					}
				}
			}
			count, total := int(jsonNumber(response.Result.ResultCount)), int(jsonNumber(response.Result.TotalCount))
			if count == 0 || offset+count > total {
				break
			}
			next := int(jsonNumber(response.Result.FirstPosition)) + count
			if next <= offset {
				return errors.New("invalid Fanza pagination")
			}
			offset = next
		}
	}
	return nil
}

func CrawlFanza(ctx context.Context, settings ClientSettings, days int, sink Sink) error {
	if settings.FanzaAPIID == "" || settings.FanzaAffiliateID == "" {
		return errors.New("Fanza credentials are not configured")
	}
	if days < 1 || days > 30 {
		return errors.New("invalid crawl range")
	}
	providers, err := NewProviders(settings)
	if err != nil {
		return err
	}
	provider := providers[0].(*httpProvider)
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return err
	}
	now := time.Now().In(location)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	seen := map[string]bool{}
	for i := 1; i <= days; i++ {
		start, end := day.AddDate(0, 0, -i), day.AddDate(0, 0, -(i-1))
		err := provider.fanzaItems(ctx, "", "date", start.Format("2006-01-02T15:04:05"), end.Format("2006-01-02T15:04:05"), func(item Parsed) error {
			if seen[item.Item.ExternalID] {
				return nil
			}
			seen[item.Item.ExternalID] = true
			provider.enrichFanza(ctx, &item)
			return sink.Save(ctx, item)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *httpProvider) enrichFanza(ctx context.Context, item *Parsed) {
	address, err := url.Parse(item.Item.SourceURL)
	if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Host == "" {
		return
	}
	host := strings.ToLower(address.Hostname())
	allowed := strings.EqualFold(address.Host, p.base.Host) || host == "dmm.co.jp" || strings.HasSuffix(host, ".dmm.co.jp") || host == "dmm.com" || strings.HasSuffix(host, ".dmm.com")
	if !allowed {
		return
	}
	body, _, err := p.get(ctx, address.String(), http.Header{"Cookie": {"age_check_done=1"}})
	if err != nil {
		return
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return
	}
	item.Item.Description = strings.TrimSpace(doc.Find(".mg-b20.lh4").First().Text())
}

func (p *httpProvider) searchSOD(ctx context.Context, id string) ([]Parsed, error) {
	var results []Parsed
	for _, variation := range catalog.ProductIDVariations(id) {
		product := p.base.ResolveReference(&url.URL{Path: "/prime/videos/", RawQuery: "id=" + url.QueryEscape(variation)})
		endpoint := p.base.ResolveReference(&url.URL{Path: "/prime/_ontime.php"})
		body, status, err := p.get(ctx, endpoint.String(), http.Header{"Cookie": {"PHPSESSID=dummy_session"}, "Referer": {product.String()}})
		if status == http.StatusNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		item, err := ParsePage("sod", product.String(), body)
		if err == nil {
			results = append(results, item)
			if item.Item.NormalizedID == id {
				break
			}
		}
	}
	return results, nil
}

func (p *httpProvider) searchMGStage(ctx context.Context, id string) ([]Parsed, error) {
	var results []Parsed
	seen := map[string]bool{}
	for _, variation := range catalog.ProductIDVariations(id) {
		for page := 1; page <= 100; page++ {
			endpoint := p.base.ResolveReference(&url.URL{Path: "/search/cSearch.php"})
			values := endpoint.Query()
			values.Set("search_word", variation)
			values.Set("page", strconv.Itoa(page))
			endpoint.RawQuery = values.Encode()
			body, _, err := p.get(ctx, endpoint.String(), http.Header{"Cookie": {"adc=1"}})
			if err != nil {
				return nil, err
			}
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
			if err != nil {
				return nil, err
			}
			links := doc.Find("div.search_list h5 a")
			if links.Length() == 0 {
				break
			}
			var urls []string
			links.Each(func(_ int, link *goquery.Selection) {
				value, ok := link.Attr("href")
				if !ok {
					return
				}
				reference, err := url.Parse(value)
				if err != nil {
					return
				}
				resolved := endpoint.ResolveReference(reference)
				if strings.EqualFold(resolved.Host, p.base.Host) && !seen[resolved.String()] {
					seen[resolved.String()] = true
					urls = append(urls, resolved.String())
				}
			})
			for _, productURL := range urls {
				body, _, err := p.get(ctx, productURL, http.Header{"Cookie": {"adc=1"}})
				if err != nil {
					return nil, err
				}
				item, err := ParsePage("mgstage", productURL, body)
				if err == nil {
					results = append(results, item)
					if item.Item.NormalizedID == id {
						return results, nil
					}
				}
			}
		}
	}
	return results, nil
}

func (p *httpProvider) searchFC2(ctx context.Context, id string) ([]Parsed, error) {
	number := strings.TrimPrefix(id, "FC2-")
	endpoint := p.base.ResolveReference(&url.URL{Path: "/article/" + number + "/"})
	body, status, err := p.get(ctx, endpoint.String(), nil)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item, err := ParsePage("fc2", endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	return []Parsed{item}, nil
}

func (p *httpProvider) get(ctx context.Context, target string, headers http.Header) ([]byte, int, error) {
	if p.limiter != nil {
		if err := p.limiter.Wait(ctx); err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, errors.New("invalid source request")
	}
	if headers != nil {
		req.Header = headers.Clone()
	}
	if p.kind == "jable" && p.settings.JableCookie != "" {
		req.Header.Set("Cookie", p.settings.JableCookie)
	}
	if p.kind == "javlibrary" && p.settings.JavLibraryCookie != "" {
		req.Header.Set("Cookie", p.settings.JavLibraryCookie)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, requestError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &RequestError{Reason: "http", Status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) > 8<<20 {
		return nil, resp.StatusCode, errors.New("source response too large or unreadable")
	}
	return body, resp.StatusCode, nil
}
