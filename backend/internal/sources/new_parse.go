package sources

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/libredmm/librefanza/backend/internal/catalog"
)

func parseNewSourcePage(source string, base *url.URL, doc *goquery.Document, item *catalog.SourceItem, metadata *Metadata) bool {
	switch source {
	case "javdb":
		item.Title = text(doc, "h2.title .current-title")
		item.CoverImageURL = attributeURL(doc, base, ".video-cover", "src")
		metadata.Samples = attributeURLs(doc, base, ".preview-images a[data-fancybox][href], .preview-images a.tile-item[href]", "href")
		doc.Find(".panel-block").Each(func(_ int, row *goquery.Selection) {
			label := strings.ToLower(strings.TrimSpace(strings.TrimRight(text(row, "strong"), ":：")))
			value := row.Find(".value").First()
			switch label {
			case "id", "番號", "番号":
				item.NormalizedID = catalog.NormalizeProductID(strings.TrimSpace(value.Text()))
			case "released date", "日期", "發行日期", "发行日期":
				item.ReleaseDate = parseDate(value.Text())
			case "duration", "時長", "时长":
				metadata.Volume = firstInt(value.Text())
			case "actor(s)", "演員", "演员":
				metadata.Cast = cast(value, "a")
			case "tags", "類別", "类别":
				metadata.Genres = texts(value, "a")
			case "maker", "片商", "製作商":
				metadata.Makers = texts(value, "a")
			case "publisher", "發行商":
				metadata.Labels = texts(value, "a")
			case "director", "導演", "导演":
				metadata.Directors = texts(value, "a")
			case "series", "系列":
				metadata.Series = texts(value, "a")
			}
		})
	case "javbooks":
		item.Title = text(doc, "#title")
		item.CoverImageURL = attributeURL(doc, base, "#info .info_cg img", "src")
		metadata.Samples = attributeURLs(doc, base, ".gallery a[href]", "href")
		doc.Find("#info .infobox").Each(func(_ int, row *goquery.Selection) {
			label := text(row, "b")
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(row.Text()), label))
			switch strings.TrimRight(label, ":：") {
			case "番號", "番号":
				item.NormalizedID = catalog.NormalizeProductID(text(row, "a"))
			case "發行時間", "发行时间":
				item.ReleaseDate = parseDate(value)
			case "影片時長", "影片时长":
				metadata.Volume = firstInt(value)
			case "女優", "女优":
				metadata.Cast = cast(row, "a")
			case "影片類別", "影片类别":
				metadata.Genres = texts(row, "a")
			case "製作商", "制作商":
				metadata.Makers = texts(row, "a")
			case "發行商", "发行商":
				metadata.Labels = texts(row, "a")
			case "導演", "导演":
				metadata.Directors = texts(row, "a")
			case "系列":
				metadata.Series = texts(row, "a")
			}
		})
	case "javquery":
		item.Title = text(doc, ".ym-detail__main h1.ym-title")
		item.NormalizedID = catalog.NormalizeProductID(text(doc, ".ym-detail__meta .ym-code"))
		item.ReleaseDate = parseDate(attributeValue(doc, ".ym-detail__meta time", "datetime"))
		item.CoverImageURL = attributeURL(doc, base, ".ym-detail__cover img", "src")
		item.Description = text(doc, ".ym-detail__desc .ym-summary")
		metadata.Samples = attributeURLs(doc, base, ".ym-detail__gallery a[href]", "href")
		doc.Find(".ym-info dt").Each(func(_ int, label *goquery.Selection) {
			value := label.NextFiltered("dd")
			switch strings.TrimSpace(label.Text()) {
			case "演员", "演員":
				metadata.Cast = cast(value, "a")
			case "类型", "類型":
				metadata.Genres = texts(value, "a")
			case "片商":
				metadata.Makers = texts(value, "a")
			case "厂牌", "廠牌":
				metadata.Labels = texts(value, "a")
			case "导演", "導演":
				metadata.Directors = texts(value, "a")
			case "时长", "時長":
				metadata.Volume = firstInt(value.Text())
				if strings.Contains(value.Text(), "秒") {
					metadata.Volume /= 60
				}
			}
		})
	case "javbus":
		item.CoverImageURL = attributeURL(doc, base, "a.bigImage", "href")
		item.ThumbnailImageURL = attributeURL(doc, base, "a.bigImage img", "src")
		metadata.Samples = attributeURLs(doc, base, "a.sample-box[href]", "href")
		metadata.Cast = cast(doc, ".star-name")
		doc.Find(".info p").Each(func(_ int, row *goquery.Selection) {
			label := text(row, ".header")
			value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(row.Text()), label))
			switch strings.TrimRight(label, ":：") {
			case "識別碼", "识别码", "識別番号":
				item.NormalizedID = catalog.NormalizeProductID(value)
			case "發行日期", "发行日期":
				item.ReleaseDate = parseDate(value)
			case "長度", "长度":
				metadata.Volume = firstInt(value)
			case "類別", "类别":
				metadata.Genres = texts(row, "a")
			case "製作商", "制作商":
				metadata.Makers = texts(row, "a")
			case "發行商", "发行商":
				metadata.Labels = texts(row, "a")
			case "導演", "导演":
				metadata.Directors = texts(row, "a")
			case "系列":
				metadata.Series = texts(row, "a")
			}
		})
		item.Title = strings.TrimSpace(strings.TrimPrefix(text(doc, "h3"), item.NormalizedID))
	default:
		return false
	}
	if item.ThumbnailImageURL == "" {
		item.ThumbnailImageURL = item.CoverImageURL
	}
	return true
}

func attributeValue(selection finder, selector, attribute string) string {
	value, _ := selection.Find(selector).First().Attr(attribute)
	return strings.TrimSpace(value)
}
