package sources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/libredmm/librefanza/backend/internal/catalog"
)

func NewCrawlSource(settings ClientSettings, name string) (CrawlSource, error) {
	providers, err := NewProviders(settings)
	if err != nil {
		return nil, err
	}
	for _, entry := range providers {
		provider := entry.(*httpProvider)
		if provider.kind == name {
			return provider, nil
		}
	}
	return nil, errors.New("invalid crawl source")
}

func (p *httpProvider) Page(ctx context.Context, page int) ([]Parsed, error) {
	if page < 1 {
		return nil, errors.New("invalid page")
	}
	if p.kind == "avmoo" {
		return p.avmooPage(ctx, page)
	}
	endpoint := &url.URL{}
	switch p.kind {
	case "libredmm":
		endpoint.Path = "/movies"
		endpoint.RawQuery = url.Values{"order": {"date_added"}, "page": {strconv.Itoa(page)}}.Encode()
	case "jable":
		endpoint.Path = "/latest-updates/"
		if page > 1 {
			endpoint.Path += strconv.Itoa(page) + "/"
		}
	case "javlibrary":
		endpoint.Path = "/ja/vl_newrelease.php"
		endpoint.RawQuery = url.Values{"page": {strconv.Itoa(page)}}.Encode()
	default:
		return nil, errors.New("invalid crawl source")
	}
	endpoint = p.base.ResolveReference(endpoint)
	body, _, err := p.get(ctx, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	if isSourceChallenge(body) {
		return nil, errors.New("source requires verification")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("invalid source page")
	}
	var links []string
	seen := map[string]bool{}
	doc.Find("a[href]").Each(func(_ int, selection *goquery.Selection) {
		href, _ := selection.Attr("href")
		reference, err := url.Parse(href)
		if err != nil {
			return
		}
		base := endpoint
		if p.kind == "javlibrary" && reference.Path == "" {
			base = p.base.ResolveReference(&url.URL{Path: "/ja/"})
		}
		resolved := base.ResolveReference(reference)
		if !strings.EqualFold(resolved.Host, p.base.Host) {
			return
		}
		valid := false
		switch p.kind {
		case "libredmm":
			id := strings.TrimPrefix(resolved.Path, "/movies/")
			valid = strings.HasPrefix(resolved.Path, "/movies/") && catalog.IsProductID(id) && !strings.Contains(id, "/")
		case "jable":
			id := strings.Trim(strings.TrimPrefix(resolved.Path, "/videos/"), "/")
			valid = strings.HasPrefix(resolved.Path, "/videos/") && catalog.IsProductID(id) && !strings.Contains(id, "/")
		case "javlibrary":
			valid = resolved.Query().Get("v") != "" && selection.ParentsFiltered(".video").Length() > 0
		}
		if valid && !seen[resolved.String()] {
			seen[resolved.String()] = true
			links = append(links, resolved.String())
		}
	})
	if len(links) > 200 {
		return nil, errors.New("source page has too many items")
	}
	items := make([]Parsed, 0, len(links))
	for _, address := range links {
		detail := address
		if p.kind == "libredmm" {
			detail += ".json"
		}
		body, _, err := p.get(ctx, detail, nil)
		if err != nil {
			return nil, err
		}
		var item Parsed
		if p.kind == "libredmm" {
			item, err = p.parseLibreDMM(body)
		} else {
			item, err = ParsePage(p.kind, address, body)
		}
		if err != nil {
			return nil, err
		}
		if p.kind == "javlibrary" && p.settings.JavLibraryCookie != "" {
			item.RawHTML = ""
		}
		items = append(items, item)
	}
	return items, nil
}

func (p *httpProvider) avmooPage(ctx context.Context, page int) ([]Parsed, error) {
	var response struct {
		Code int          `json:"code"`
		Data []avmooMovie `json:"data"`
	}
	if err := p.postJSON(ctx, "/jav/data/api/getMovies", []any{map[string]string{"lang": "ja"}, 60, page}, &response); err != nil {
		return nil, err
	}
	if response.Code != 200 || response.Data == nil || len(response.Data) > 60 {
		return nil, errors.New("invalid source response")
	}
	items := make([]Parsed, 0, len(response.Data))
	for _, movie := range response.Data {
		if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(movie.ID) {
			return nil, errors.New("invalid source item")
		}
		var detail struct {
			Code int        `json:"code"`
			Data avmooMovie `json:"data"`
		}
		if err := p.postJSON(ctx, "/jav/data/api/getMovie", []any{movie.ID, "ja"}, &detail); err != nil {
			return nil, err
		}
		if detail.Code != 200 || detail.Data.ID != movie.ID {
			return nil, fmt.Errorf("invalid source detail")
		}
		item, err := p.parseAVMOO(detail.Data)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}
