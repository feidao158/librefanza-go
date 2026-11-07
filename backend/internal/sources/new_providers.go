package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func (p *httpProvider) searchNewSource(ctx context.Context, id string) ([]Parsed, error) {
	var endpoint *url.URL
	var body []byte
	var err error
	switch p.kind {
	case "javdb":
		endpoint = p.base.ResolveReference(&url.URL{Path: "/search", RawQuery: url.Values{"q": {id}, "f": {"all"}}.Encode()})
		body, _, err = p.get(ctx, endpoint.String(), nil)
	case "javbus":
		endpoint = p.base.ResolveReference(&url.URL{Path: "/search/" + id})
		body, _, err = p.get(ctx, endpoint.String(), nil)
	case "javbooks":
		endpoint = p.base.ResolveReference(&url.URL{Path: "/serch_censored.htm"})
		body, _, err = p.request(ctx, http.MethodPost, endpoint.String(), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, []byte(url.Values{"skey": {id}}.Encode()))
		if browserFallbackAllowed(err) {
			body, endpoint, err = p.fetchBrowser(ctx, p.base.String(), id)
		}
	case "javquery":
		body, endpoint, err = p.searchRendered(ctx, id)
	default:
		return nil, errors.New("unsupported source")
	}
	if err != nil {
		return nil, err
	}
	links, err := p.newSourceLinks(endpoint, body)
	if err != nil {
		return nil, err
	}
	for _, address := range links {
		item, err := p.newSourceDetail(ctx, address)
		if err != nil {
			if errors.Is(err, errInvalidProduct) {
				continue
			}
			return nil, err
		}
		if item.Item.NormalizedID == id {
			return []Parsed{item}, nil
		}
	}
	return nil, nil
}

var errInvalidProduct = errors.New("source page has no valid product")

func (p *httpProvider) newSourceDetail(ctx context.Context, address string) (Parsed, error) {
	var body []byte
	var err error
	if p.kind == "javquery" {
		body, err = p.renderPage(ctx, address)
	} else {
		var status int
		body, status, err = p.get(ctx, address, nil)
		if status == http.StatusNotFound || status == http.StatusGone {
			return Parsed{}, errInvalidProduct
		}
		if p.kind == "javbooks" && browserFallbackAllowed(err) {
			body, _, err = p.fetchBrowser(ctx, address, "")
		}
	}
	if err != nil {
		var request *RequestError
		if errors.As(err, &request) && (request.Status == http.StatusNotFound || request.Status == http.StatusGone) {
			return Parsed{}, errInvalidProduct
		}
		return Parsed{}, err
	}
	if isSourceChallenge(body) {
		return Parsed{}, errors.New("source requires verification")
	}
	item, err := ParsePage(p.kind, address, body)
	if err != nil && err.Error() == errInvalidProduct.Error() {
		doc, parseErr := goquery.NewDocumentFromReader(bytes.NewReader(body))
		if parseErr != nil {
			return Parsed{}, errors.New("invalid source page")
		}
		selector := map[string]string{"javdb": ".panel-block", "javbus": ".info", "javbooks": "#info", "javquery": ".ym-detail__main"}[p.kind]
		if selector == "" || doc.Find(selector).Length() == 0 {
			return Parsed{}, errors.New("invalid source page")
		}
		return Parsed{}, errInvalidProduct
	}
	if err != nil {
		return Parsed{}, err
	}
	if p.sourceCookie() != "" || p.kind == "javquery" {
		item.RawHTML = ""
	}
	return item, nil
}

func browserFallbackAllowed(err error) bool {
	var request *RequestError
	return errors.As(err, &request) && request.Status == 0 && request.Reason != "cancelled"
}

func (p *httpProvider) fetchBrowser(ctx context.Context, address, search string) ([]byte, *url.URL, error) {
	if p.browserFetch != nil {
		return p.browserFetch(ctx, address, search)
	}
	return p.browser(ctx, address, search)
}

func (p *httpProvider) sourceCookie() string {
	return map[string]string{"jable": p.settings.JableCookie, "javdb": p.settings.JavDBCookie, "javbus": p.settings.JavBusCookie, "javbooks": p.settings.JavBooksCookie, "javquery": p.settings.JavQueryCookie}[p.kind]
}

func (p *httpProvider) newSourceLinks(endpoint *url.URL, body []byte) ([]string, error) {
	if isSourceChallenge(body) {
		return nil, errors.New("source requires verification")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid source page")
	}
	selector, container := "", ""
	switch p.kind {
	case "javdb":
		selector, container = `.movie-list a[href^="/v/"]`, ".movie-list, .empty-message, .empty"
	case "javbus":
		selector, container = `a.movie-box[href]`, "#waterfall, .movie-box, .alert"
	case "javbooks":
		selector = `a[href*="/content_censored/"]`
	case "javquery":
		selector, container = `.ym-card a[href*="/post/"], .ym-card a[href*="/fc2-post/"], .ym-card a[href*="/uncensored-post/"]`, ".ym-search-results, .ym-grid, .ym-block"
	}
	seen := map[string]bool{}
	var links []string
	tooMany := false
	doc.Find(selector).Each(func(_ int, link *goquery.Selection) {
		value, _ := link.Attr("href")
		ref, err := url.Parse(value)
		if err != nil || ref.User != nil {
			return
		}
		address := endpoint.ResolveReference(ref)
		if !strings.EqualFold(address.Host, p.base.Host) || (address.Scheme != "https" && address.Scheme != "http") || seen[address.String()] {
			return
		}
		seen[address.String()] = true
		if len(links) < 100 {
			links = append(links, address.String())
		} else {
			tooMany = true
		}
	})
	if tooMany {
		return nil, errors.New("source page has too many items")
	}
	if len(links) == 0 {
		if p.kind == "javbooks" {
			content := strings.ReplaceAll(doc.Text(), " ", "")
			if !strings.Contains(content, "搜尋到0筆資料") && !strings.Contains(content, "搜索到0笔资料") {
				return nil, errors.New("invalid source page")
			}
		} else if container == "" || doc.Find(container).Length() == 0 {
			return nil, errors.New("invalid source page")
		}
	}
	return links, nil
}

func (p *httpProvider) newSourcePageLinks(ctx context.Context, page int) ([]string, error) {
	var endpoint *url.URL
	switch p.kind {
	case "javdb":
		endpoint = p.base.ResolveReference(&url.URL{Path: "/", RawQuery: url.Values{"page": {strconv.Itoa(page)}}.Encode()})
	case "javbus":
		endpoint = p.base.ResolveReference(&url.URL{Path: "/page/" + strconv.Itoa(page)})
	case "javbooks":
		endpoint = p.base.ResolveReference(&url.URL{Path: fmt.Sprintf("/serchinfo_censored/topicsbt/topicsbt_%d.htm", page)})
	case "javquery":
		endpoint = p.base.ResolveReference(&url.URL{Path: "/jav.html", RawQuery: url.Values{"page": {strconv.Itoa(page)}}.Encode()})
	default:
		return nil, errors.New("unsupported source")
	}
	var body []byte
	var err error
	if p.kind == "javquery" {
		body, err = p.renderPage(ctx, endpoint.String())
	} else {
		body, _, err = p.get(ctx, endpoint.String(), nil)
		if p.kind == "javbooks" && browserFallbackAllowed(err) {
			body, _, err = p.fetchBrowser(ctx, endpoint.String(), "")
		}
	}
	if err != nil {
		return nil, err
	}
	links, err := p.newSourceLinks(endpoint, body)
	if err != nil {
		return nil, err
	}
	return links, nil
}

func (p *httpProvider) newSourcePage(ctx context.Context, page int) ([]Parsed, int, error) {
	links, err := p.newSourcePageLinks(ctx, page)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Parsed, 0, len(links))
	skipped := 0
	for _, address := range links {
		item, err := p.newSourceDetail(ctx, address)
		if errors.Is(err, errInvalidProduct) {
			skipped++
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, skipped, nil
}

func ProbeSource(ctx context.Context, settings ClientSettings, name string) (Parsed, error) {
	switch name {
	case "javdb", "javbus", "javbooks", "javquery":
	default:
		return Parsed{}, errors.New("invalid probe source")
	}
	provider, err := NewCrawlSource(settings, name)
	if err != nil {
		return Parsed{}, err
	}
	p := provider.(*httpProvider)
	items, err := p.withHealth(ctx, func() ([]Parsed, error) {
		links, err := p.newSourcePageLinks(ctx, 1)
		if err != nil {
			return nil, err
		}
		for index, address := range links {
			if index >= 3 {
				break
			}
			item, err := p.newSourceDetail(ctx, address)
			if errors.Is(err, errInvalidProduct) {
				continue
			}
			if err != nil {
				return nil, err
			}
			return []Parsed{item}, nil
		}
		return nil, errors.New("invalid source page: no valid product")
	})
	if err != nil {
		return Parsed{}, err
	}
	return items[0], nil
}
