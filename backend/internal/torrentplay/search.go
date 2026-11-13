package torrentplay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type Result struct {
	Title   string
	Magnet  string
	Seeders int
	Page    string
}

func cleanMagnet(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "magnet" {
		return ""
	}
	params := parsed.Query()
	xt := params.Get("xt")
	if !regexp.MustCompile(`(?i)^urn:btih:[a-f0-9]{40}$`).MatchString(xt) {
		return ""
	}
	allowed := map[string]bool{"sukebei.tracker.wf": true, "open.stealth.si": true, "tracker.opentrackr.org": true, "exodus.desync.com": true, "tracker.torrent.eu.org": true}
	clean := url.Values{"xt": []string{xt}}
	for _, item := range params["tr"] {
		tracker, err := url.Parse(item)
		if err == nil && tracker.Scheme == "udp" && tracker.User == nil && allowed[strings.ToLower(tracker.Hostname())] && tracker.Port() != "" && tracker.Path == "/announce" {
			clean.Add("tr", tracker.String())
		}
	}
	return "magnet:?" + clean.Encode()
}

// Search 查询并按做种数降序返回番号完全匹配的候选。
func Search(ctx context.Context, client *http.Client, base, id string) ([]Result, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)+$`).MatchString(id) {
		return nil, errors.New("invalid product ID")
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	query := baseURL.Query()
	query.Set("q", strings.ToUpper(id))
	query.Set("c", "2_2")
	query.Set("s", "seeders")
	query.Set("o", "desc")
	baseURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("search unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, errors.New("invalid search response")
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(`(?i)(?:^|[^a-z0-9])` + regexp.QuoteMeta(id) + `(?:$|[^a-z0-9])`)
	results := make([]Result, 0)
	doc.Find("table.torrent-list tbody tr").Each(func(_ int, row *goquery.Selection) {
		cols := row.Find("td")
		if cols.Length() < 8 {
			return
		}
		a := cols.Eq(1).Find(`a[href^="/view/"]`).Last()
		title, _ := a.Attr("title")
		if title == "" {
			title = strings.TrimSpace(a.Text())
		}
		if !pattern.MatchString(title) {
			return
		}
		href, ok := row.Find(`a[href^="magnet:"]`).First().Attr("href")
		if !ok {
			return
		}
		magnet := cleanMagnet(href)
		if magnet == "" {
			return
		}
		seeders, err := strconv.Atoi(strings.TrimSpace(cols.Eq(cols.Length() - 3).Text()))
		if err != nil || seeders < 0 {
			return
		}
		page, _ := a.Attr("href")
		results = append(results, Result{Title: title, Magnet: magnet, Seeders: seeders, Page: "https://sukebei.nyaa.si" + page})
	})
	sort.SliceStable(results, func(i, j int) bool { return results[i].Seeders > results[j].Seeders })
	return results, nil
}
