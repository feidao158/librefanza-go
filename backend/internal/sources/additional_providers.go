package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/libredmm/librefanza/backend/internal/catalog"
)

type remotePerson struct {
	Name     string `json:"name"`
	StarName string `json:"starName"`
}

type avmooMovie struct {
	ID          string         `json:"movieId"`
	Code        string         `json:"movieFanHao"`
	Title       string         `json:"title"`
	Japanese    string         `json:"title_ja"`
	ReleaseDate string         `json:"releaseDate"`
	Length      int            `json:"length"`
	Poster      string         `json:"posterLarge"`
	Thumbnail   string         `json:"posterSmall"`
	Samples     []string       `json:"sampleLarge"`
	Stars       []remotePerson `json:"star"`
	Genres      []struct {
		Name string `json:"genreName"`
	} `json:"genre"`
	Label struct {
		Name string `json:"labelName"`
	} `json:"label"`
	Studio struct {
		Name string `json:"studioName"`
	} `json:"studio"`
}

func (p *httpProvider) postJSON(ctx context.Context, path string, input, output any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return errors.New("invalid source request")
	}
	target := p.base.ResolveReference(&url.URL{Path: path})
	data, _, err := p.request(ctx, http.MethodPost, target.String(), http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json, text/plain, */*"}}, body)
	if err != nil {
		return err
	}
	if json.Unmarshal(data, output) != nil {
		return errors.New("invalid source response")
	}
	return nil
}

func (p *httpProvider) searchAVMOO(ctx context.Context, id string) ([]Parsed, error) {
	var response struct {
		Code int          `json:"code"`
		Data []avmooMovie `json:"data"`
	}
	if err := p.postJSON(ctx, "/jav/data/api/search", []any{map[string]string{"search": id, "lang": "ja"}, 60, 1}, &response); err != nil {
		return nil, err
	}
	if response.Code != 200 {
		return nil, errors.New("source returned an error")
	}
	for _, match := range response.Data {
		if catalog.NormalizeProductID(match.Code) != id || !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(match.ID) {
			continue
		}
		var detail struct {
			Code int        `json:"code"`
			Data avmooMovie `json:"data"`
		}
		if err := p.postJSON(ctx, "/jav/data/api/getMovie", []any{match.ID, "ja"}, &detail); err != nil {
			return nil, err
		}
		if detail.Code != 200 || detail.Data.ID != match.ID || catalog.NormalizeProductID(detail.Data.Code) != id {
			return nil, errors.New("invalid source item")
		}
		item, err := p.parseAVMOO(detail.Data)
		if err != nil {
			return nil, err
		}
		return []Parsed{item}, nil
	}
	return nil, nil
}

func (p *httpProvider) parseAVMOO(movie avmooMovie) (Parsed, error) {
	title := strings.TrimSpace(movie.Title)
	if title == "" {
		title = strings.TrimSpace(movie.Japanese)
	}
	id := catalog.NormalizeProductID(movie.Code)
	if title == "" || !catalog.IsProductID(id) {
		return Parsed{}, errors.New("invalid source item")
	}
	metadata := Metadata{Volume: movie.Length, Samples: movie.Samples}
	for _, star := range movie.Stars {
		if name := strings.TrimSpace(star.StarName); name != "" {
			metadata.Cast = append(metadata.Cast, CastMember{Name: name})
		}
	}
	for _, genre := range movie.Genres {
		if name := strings.TrimSpace(genre.Name); name != "" {
			metadata.Genres = append(metadata.Genres, name)
		}
	}
	if name := strings.TrimSpace(movie.Label.Name); name != "" {
		metadata.Labels = append(metadata.Labels, name)
	}
	if name := strings.TrimSpace(movie.Studio.Name); name != "" {
		metadata.Makers = append(metadata.Makers, name)
	}
	encoded, _ := json.Marshal(metadata)
	raw, _ := json.Marshal(movie)
	address := p.base.ResolveReference(&url.URL{Path: "/ja/movies/" + movie.ID})
	return Parsed{Item: catalog.SourceItem{Source: "avmoo", ExternalID: movie.ID, NormalizedID: id, Title: title, ReleaseDate: parseDate(movie.ReleaseDate), SourceURL: address.String(), CoverImageURL: movie.Poster, ThumbnailImageURL: movie.Thumbnail, MetadataJSON: string(encoded), RawJSON: string(raw)}, Cast: metadata.Cast}, nil
}

func (p *httpProvider) searchLibreDMM(ctx context.Context, id string) ([]Parsed, error) {
	endpoint := p.base.ResolveReference(&url.URL{Path: "/movies"})
	query := endpoint.Query()
	query.Set("q", id)
	endpoint.RawQuery = query.Encode()
	body, _, err := p.get(ctx, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil || isSourceChallenge(body) {
		return nil, errors.New("invalid source page")
	}
	found := false
	doc.Find("a[href]").EachWithBreak(func(_ int, link *goquery.Selection) bool {
		href, _ := link.Attr("href")
		reference, err := url.Parse(href)
		if err == nil && (reference.Host == "" || strings.EqualFold(reference.Host, p.base.Host)) && strings.EqualFold(reference.Path, "/movies/"+id) {
			found = true
			return false
		}
		return true
	})
	if !found {
		return nil, nil
	}
	detailURL := p.base.ResolveReference(&url.URL{Path: "/movies/" + id + ".json"})
	body, _, err = p.get(ctx, detailURL.String(), nil)
	if err != nil {
		return nil, err
	}
	item, err := p.parseLibreDMM(body)
	if err != nil || item.Item.NormalizedID != id {
		return nil, errors.New("invalid source item")
	}
	return []Parsed{item}, nil
}

func (p *httpProvider) parseLibreDMM(body []byte) (Parsed, error) {
	var movie struct {
		ID          string         `json:"normalized_id"`
		Title       string         `json:"title"`
		Date        string         `json:"date"`
		Description string         `json:"description"`
		Cover       string         `json:"cover_image_url"`
		Thumbnail   string         `json:"thumbnail_image_url"`
		Volume      int            `json:"volume"`
		Rating      float64        `json:"review"`
		Actresses   []remotePerson `json:"actresses"`
		Genres      []string       `json:"genres"`
		Labels      []string       `json:"labels"`
		Makers      []string       `json:"makers"`
		Directors   []string       `json:"directors"`
		Samples     []string       `json:"sample_image_urls"`
	}
	if json.Unmarshal(body, &movie) != nil {
		return Parsed{}, errors.New("invalid source response")
	}
	id, title := catalog.NormalizeProductID(movie.ID), strings.TrimSpace(movie.Title)
	if !catalog.IsProductID(id) || title == "" {
		return Parsed{}, errors.New("invalid source item")
	}
	metadata := Metadata{Volume: movie.Volume / 60, Rating: movie.Rating, Genres: movie.Genres, Labels: movie.Labels, Makers: movie.Makers, Directors: movie.Directors, Samples: movie.Samples}
	for _, actress := range movie.Actresses {
		if name := strings.TrimSpace(actress.Name); name != "" {
			metadata.Cast = append(metadata.Cast, CastMember{Name: name})
		}
	}
	encoded, _ := json.Marshal(metadata)
	address := p.base.ResolveReference(&url.URL{Path: "/movies/" + id})
	return Parsed{Item: catalog.SourceItem{Source: "libredmm", ExternalID: id, NormalizedID: id, Title: title, ReleaseDate: parseDate(movie.Date), Description: strings.TrimSpace(movie.Description), SourceURL: address.String(), CoverImageURL: movie.Cover, ThumbnailImageURL: movie.Thumbnail, MetadataJSON: string(encoded), RawJSON: string(body)}, Cast: metadata.Cast}, nil
}

func ParseRemoteJSON(source, address string, body []byte) (Parsed, error) {
	base, err := url.Parse(address)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return Parsed{}, errors.New("invalid source URL")
	}
	p := &httpProvider{kind: source, base: base}
	switch source {
	case "avmoo":
		var movie avmooMovie
		if json.Unmarshal(body, &movie) != nil || !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(movie.ID) {
			return Parsed{}, errors.New("invalid source item")
		}
		return p.parseAVMOO(movie)
	case "libredmm":
		return p.parseLibreDMM(body)
	default:
		return Parsed{}, errors.New("unsupported source")
	}
}

func (p *httpProvider) searchJavLibrary(ctx context.Context, id string) ([]Parsed, error) {
	endpoint := p.base.ResolveReference(&url.URL{Path: "/ja/vl_searchbyid.php"})
	query := endpoint.Query()
	query.Set("keyword", id)
	endpoint.RawQuery = query.Encode()
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
	if doc.Find("#video_id td.text").Length() > 0 {
		item, err := ParsePage("javlibrary", endpoint.String(), body)
		if err != nil || item.Item.NormalizedID != id {
			return nil, err
		}
		if p.settings.JavLibraryCookie != "" {
			item.RawHTML = ""
		}
		return []Parsed{item}, nil
	}
	var links []string
	doc.Find(".video a[href]").Each(func(_ int, link *goquery.Selection) {
		href, _ := link.Attr("href")
		reference, err := url.Parse(href)
		if err != nil {
			return
		}
		base := endpoint
		if reference.Path == "" {
			base = p.base.ResolveReference(&url.URL{Path: "/ja/"})
		}
		resolved := base.ResolveReference(reference)
		if strings.EqualFold(resolved.Host, p.base.Host) && resolved.Query().Get("v") != "" && len(links) < 20 {
			links = append(links, resolved.String())
		}
	})
	for _, address := range links {
		body, _, err := p.get(ctx, address, nil)
		if err != nil {
			return nil, err
		}
		item, err := ParsePage("javlibrary", address, body)
		if err == nil && item.Item.NormalizedID == id {
			if p.settings.JavLibraryCookie != "" {
				item.RawHTML = ""
			}
			return []Parsed{item}, nil
		}
	}
	return nil, nil
}

func (p *httpProvider) searchJable(ctx context.Context, id string) ([]Parsed, error) {
	endpoint := p.base.ResolveReference(&url.URL{Path: "/videos/" + strings.ToLower(id) + "/"})
	body, status, err := p.get(ctx, endpoint.String(), nil)
	if status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item, err := ParsePage("jable", endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	return []Parsed{item}, nil
}

func FetchJablePage(ctx context.Context, settings ClientSettings, id string) ([]byte, string, error) {
	if !catalog.IsProductID(id) || catalog.NormalizeProductID(id) != id || !regexp.MustCompile(`^[A-Z0-9-]+$`).MatchString(id) {
		return nil, "", errors.New("invalid product ID")
	}
	source, err := NewCrawlSource(settings, "jable")
	if err != nil {
		return nil, "", err
	}
	p := source.(*httpProvider)
	endpoint := p.base.ResolveReference(&url.URL{Path: "/videos/" + strings.ToLower(id) + "/"})
	body, _, err := p.get(ctx, endpoint.String(), nil)
	if err != nil {
		return nil, "", err
	}
	if isSourceChallenge(body) {
		return nil, "", &RequestError{Reason: "challenge", Status: http.StatusOK}
	}
	return body, endpoint.String(), nil
}

func isSourceChallenge(body []byte) bool {
	lower := bytes.ToLower(body)
	return bytes.Contains(lower, []byte("challenge-form")) || bytes.Contains(lower, []byte("access denied")) || bytes.Contains(lower, []byte("just a moment")) || bytes.Contains(lower, []byte("driver-verify")) || bytes.Contains(lower, []byte("age verification javbus"))
}
