package sources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/libredmm/librefanza/backend/internal/catalog"
)

type CastMember struct {
	FanzaID *int64 `json:"fanza_id,omitempty"`
	Name    string `json:"name"`
}

type Metadata struct {
	Cast      []CastMember `json:"cast"`
	Genres    []string     `json:"genres"`
	Labels    []string     `json:"labels"`
	Makers    []string     `json:"makers"`
	Directors []string     `json:"directors"`
	Samples   []string     `json:"sample_image_urls"`
	Volume    int          `json:"volume_minutes,omitempty"`
	Rating    float64      `json:"rating,omitempty"`
}

type Parsed struct {
	Item    catalog.SourceItem
	Cast    []CastMember
	RawHTML string
}

type namedValue struct {
	ID   json.RawMessage `json:"id"`
	Name string          `json:"name"`
}

func ParseFanza(body []byte) (Parsed, error) {
	var data struct {
		ContentID    string          `json:"content_id"`
		MakerProduct string          `json:"maker_product"`
		Title        string          `json:"title"`
		Date         string          `json:"date"`
		URL          string          `json:"URL"`
		Volume       json.RawMessage `json:"volume"`
		ImageURL     struct {
			Large string `json:"large"`
			Small string `json:"small"`
		} `json:"imageURL"`
		SampleImageURL struct {
			Large struct {
				Image []string `json:"image"`
			} `json:"sample_l"`
			Medium struct {
				Image []string `json:"image"`
			} `json:"sample_m"`
			Small struct {
				Image []string `json:"image"`
			} `json:"sample_s"`
		} `json:"sampleImageURL"`
		Review struct {
			Average json.RawMessage `json:"average"`
		} `json:"review"`
		ItemInfo struct {
			Actress  []namedValue `json:"actress"`
			Genre    []namedValue `json:"genre"`
			Label    []namedValue `json:"label"`
			Maker    []namedValue `json:"maker"`
			Director []namedValue `json:"director"`
		} `json:"iteminfo"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return Parsed{}, errors.New("invalid source JSON")
	}
	contentID, title := strings.TrimSpace(data.ContentID), strings.TrimSpace(data.Title)
	id := catalog.NormalizeProductID(contentID)
	if id == contentID && data.MakerProduct != "" {
		id = catalog.NormalizeProductID(strings.TrimSpace(data.MakerProduct))
	}
	if contentID == "" || title == "" || !catalog.IsProductID(id) {
		return Parsed{}, errors.New("invalid source item")
	}
	metadata := Metadata{Genres: names(data.ItemInfo.Genre), Labels: names(data.ItemInfo.Label), Makers: names(data.ItemInfo.Maker), Directors: names(data.ItemInfo.Director)}
	for _, actress := range data.ItemInfo.Actress {
		name := strings.TrimSpace(actress.Name)
		if name != "" {
			entry := CastMember{Name: name}
			if id := int64(jsonNumber(actress.ID)); id > 0 {
				entry.FanzaID = &id
			}
			metadata.Cast = append(metadata.Cast, entry)
		}
	}
	metadata.Samples = data.SampleImageURL.Large.Image
	if len(metadata.Samples) == 0 {
		metadata.Samples = data.SampleImageURL.Medium.Image
	}
	if len(metadata.Samples) == 0 {
		metadata.Samples = data.SampleImageURL.Small.Image
	}
	metadata.Volume = int(jsonNumber(data.Volume))
	metadata.Rating = jsonNumber(data.Review.Average)
	metadata.Rating *= 2
	encoded, _ := json.Marshal(metadata)
	cover := data.ImageURL.Large
	if cover == "" {
		cover = data.ImageURL.Small
	}
	return Parsed{Item: catalog.SourceItem{Source: "fanza", ExternalID: contentID, NormalizedID: id, Title: title, ReleaseDate: parseDate(data.Date), SourceURL: data.URL, CoverImageURL: cover, ThumbnailImageURL: data.ImageURL.Small, MetadataJSON: string(encoded), RawJSON: string(body)}, Cast: metadata.Cast}, nil
}

func ParsePage(source, sourceURL string, body []byte) (Parsed, error) {
	base, err := url.Parse(sourceURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return Parsed{}, errors.New("invalid source URL")
	}
	if len(body) == 0 || isSourceChallenge(body) {
		return Parsed{}, errors.New("invalid source page")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return Parsed{}, err
	}
	item := catalog.SourceItem{Source: source, SourceURL: sourceURL}
	metadata := Metadata{}
	switch source {
	case "javlibrary":
		id := text(doc, "#video_id td.text")
		item.NormalizedID = catalog.NormalizeProductID(id)
		item.Title = strings.TrimSpace(strings.TrimPrefix(text(doc, "#video_title > h3"), id))
		item.ReleaseDate = parseDate(text(doc, "#video_date td.text"))
		item.CoverImageURL = attributeURL(doc, base, "#video_jacket_img", "src")
		item.ThumbnailImageURL = item.CoverImageURL
		if imageURL, err := url.Parse(item.CoverImageURL); err == nil && strings.EqualFold(imageURL.Hostname(), "pics.dmm.co.jp") && strings.HasSuffix(imageURL.Path, "pl.jpg") {
			imageURL.Path = strings.TrimSuffix(imageURL.Path, "pl.jpg") + "ps.jpg"
			item.ThumbnailImageURL = imageURL.String()
		}
		metadata.Cast = cast(doc, ".cast .star")
		metadata.Genres = texts(doc, "span.genre")
		metadata.Labels = texts(doc, "span.label")
		metadata.Makers = texts(doc, "span.maker")
		metadata.Directors = texts(doc, "span.director")
		metadata.Volume = firstInt(text(doc, "#video_length span.text"))
	case "mgstage":
		item.Title = text(doc, "h1.tag")
		item.CoverImageURL = attributeURL(doc, base, "#EnlargeImage", "href")
		item.ThumbnailImageURL = attributeURL(doc, base, ".enlarge_image", "src")
		item.Description = text(doc, "div.introduction")
		doc.Find(".detail_data table tr").Each(func(_ int, row *goquery.Selection) {
			label, value := text(row, "th"), row.Find("td")
			switch {
			case strings.HasPrefix(label, "品番"):
				item.NormalizedID = catalog.NormalizeProductID(strings.TrimSpace(value.Text()))
			case strings.HasPrefix(label, "出演"):
				metadata.Cast = cast(value, "a")
			case strings.HasPrefix(label, "配信開始日"):
				item.ReleaseDate = parseDate(strings.TrimSpace(value.Text()))
			case strings.HasPrefix(label, "ジャンル"):
				metadata.Genres = texts(value, "a")
			case strings.HasPrefix(label, "レーベル"):
				metadata.Labels = texts(value, "a")
			case strings.HasPrefix(label, "メーカー"):
				metadata.Makers = texts(value, "a")
			case strings.HasPrefix(label, "収録時間"):
				metadata.Volume = firstInt(value.Text())
			}
		})
	case "sod":
		item.Title = text(doc, "#videos_head h1:not([style*='none'])")
		item.CoverImageURL = attributeURL(doc, base, "div.videos_samimg > a", "href")
		item.ThumbnailImageURL = attributeURL(doc, base, "div.videos_samimg > a > img", "src")
		item.Description = text(doc, "div.videos_textli > article")
		doc.Find("table#v_introduction tr").Each(func(_ int, row *goquery.Selection) {
			label, value := text(row, "td.v_intr_ti"), row.Find("td.v_intr_tx")
			switch {
			case strings.HasPrefix(label, "品番"):
				item.NormalizedID = catalog.NormalizeProductID(strings.TrimSpace(value.Text()))
			case strings.HasPrefix(label, "出演者"):
				metadata.Cast = cast(value, "a")
			case strings.HasPrefix(label, "発売年月日"):
				item.ReleaseDate = parseDate(strings.TrimSpace(value.Text()))
			case strings.HasPrefix(label, "ジャンル"):
				metadata.Genres = texts(value, "a")
			case strings.HasPrefix(label, "レーベル"):
				metadata.Labels = texts(value, "a")
			case strings.HasPrefix(label, "メーカー"):
				metadata.Makers = texts(value, "a")
			case strings.HasPrefix(label, "監督"):
				metadata.Directors = texts(value, "a")
			case strings.HasPrefix(label, "再生時間"):
				metadata.Volume = firstInt(value.Text())
			}
		})
	case "fc2":
		number := path.Base(strings.TrimRight(base.Path, "/"))
		if !regexp.MustCompile(`^[0-9]+$`).MatchString(number) {
			return Parsed{}, errors.New("invalid FC2 URL")
		}
		item.NormalizedID = "FC2-" + number
		item.Title = text(doc, ".items_article_headerInfo > h3")
		item.CoverImageURL = attributeURL(doc, base, ".items_article_SampleImages li a", "href")
		item.ThumbnailImageURL = attributeURL(doc, base, ".items_article_MainitemThumb img", "src")
		item.ReleaseDate = parseDate(text(doc, "div.items_article_Releasedate"))
		metadata.Labels = texts(doc, ".items_article_TagArea a.tag")
	case "jable":
		item.NormalizedID = catalog.NormalizeProductID(path.Base(strings.TrimRight(base.Path, "/")))
		item.Title = text(doc, "h4")
		if item.Title == "" {
			item.Title, _ = doc.Find(`meta[property="og:title"]`).Attr("content")
			item.Title = strings.TrimSpace(item.Title)
		}
		item.CoverImageURL = attributeURL(doc, base, `meta[property="og:image"]`, "content")
		item.ThumbnailImageURL = item.CoverImageURL
		metadata.Cast = cast(doc, `a[href*="/models/"]`)
	default:
		return Parsed{}, errors.New("unsupported source")
	}
	if item.Title == "" || !catalog.IsProductID(item.NormalizedID) {
		return Parsed{}, errors.New("source page has no valid product")
	}
	key := sha256.Sum256([]byte(sourceURL))
	item.ExternalID = hex.EncodeToString(key[:])
	encoded, _ := json.Marshal(metadata)
	item.MetadataJSON = string(encoded)
	rawHTML := string(body)
	if source == "jable" {
		rawHTML = ""
	}
	return Parsed{Item: item, Cast: metadata.Cast, RawHTML: rawHTML}, nil
}

func names(values []namedValue) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if name := strings.TrimSpace(value.Name); name != "" {
			result = append(result, name)
		}
	}
	return result
}

type finder interface {
	Find(string) *goquery.Selection
}

func text(selection finder, selector string) string {
	return strings.TrimSpace(selection.Find(selector).First().Text())
}

func texts(selection finder, selector string) []string {
	values := []string{}
	selection.Find(selector).Each(func(_ int, item *goquery.Selection) {
		if value := strings.TrimSpace(item.Text()); value != "" {
			values = append(values, value)
		}
	})
	return values
}

func cast(selection finder, selector string) []CastMember {
	result := []CastMember{}
	for _, name := range texts(selection, selector) {
		result = append(result, CastMember{Name: name})
	}
	return result
}

func attributeURL(selection finder, base *url.URL, selector, attribute string) string {
	value, exists := selection.Find(selector).First().Attr(attribute)
	if !exists {
		return ""
	}
	reference, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	if resolved.Scheme == "http" && strings.EqualFold(resolved.Hostname(), "pics.dmm.co.jp") {
		resolved.Scheme = "https"
	}
	return resolved.String()
}

func parseDate(value string) *time.Time {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("年", "-", "月", "-", "日", "", "/", "-").Replace(value)
	for _, format := range []string{time.RFC3339, "2006-01-02 15:04:05 -0700", "2006-01-02 15:04:05", "2006-01-02", "2006-1-2"} {
		if date, err := time.Parse(format, value); err == nil {
			return &date
		}
	}
	return nil
}

func firstInt(value string) int {
	match := regexp.MustCompile(`[0-9]+`).FindString(value)
	result, _ := strconv.Atoi(match)
	return result
}

func jsonNumber(value json.RawMessage) float64 {
	text := strings.Trim(string(value), `"`)
	result, _ := strconv.ParseFloat(text, 64)
	return result
}
