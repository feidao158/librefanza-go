package sources

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExistingSourceFixtures(t *testing.T) {
	tests := []struct {
		source string
		url    string
		file   string
		id     string
	}{
		{"javlibrary", "http://www.javlibrary.com/ja/?v=javlio354y", "javlibrary.html", "ABP-001"},
		{"mgstage", "https://www.mgstage.com/product/product_detail/261ARA-168/", "mgstage.html", "ARA-168"},
		{"sod", "https://ec.sod.co.jp/prime/videos/?id=STARS-455", "sod.html", "STARS-455"},
		{"fc2", "https://adult.contents.fc2.com/article/3069309/", "fc2.html", "FC2-3069309"},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("testdata", test.file))
			if err != nil {
				t.Fatal(err)
			}
			item, err := ParsePage(test.source, test.url, body)
			if err != nil || item.Item.NormalizedID != test.id || item.Item.Title == "" || item.Item.Source != test.source {
				t.Fatalf("item=%+v err=%v", item.Item, err)
			}
			if test.source == "javlibrary" && (!strings.HasPrefix(item.Item.CoverImageURL, "https://") || !strings.HasSuffix(item.Item.ThumbnailImageURL, "ps.jpg")) {
				t.Fatalf("JavLibrary cover URLs invalid: %s %s", item.Item.CoverImageURL, item.Item.ThumbnailImageURL)
			}
		})
	}
}

func TestParseFanzaRemovesUnicodeWhitespace(t *testing.T) {
	body := []byte(`{"content_id":"　abp00123　","title":"　标题　","date":"2024-01-02 10:00:00","URL":"https://example.com/item","imageURL":{"large":"https://example.com/large.jpg"},"iteminfo":{"actress":[{"id":12,"name":"　演员　"}]}}`)
	item, err := ParseFanza(body)
	if err != nil || item.Item.NormalizedID != "ABP-123" || item.Item.Title != "标题" || len(item.Cast) != 1 || item.Cast[0].Name != "演员" {
		t.Fatalf("item=%+v err=%v", item, err)
	}
}

func TestRejectInvalidPage(t *testing.T) {
	if _, err := ParsePage("javlibrary", "https://example.com", []byte("<html>challenge-form</html>")); err == nil {
		t.Fatal("challenge page was accepted")
	}
}

func TestParseFanzaAcceptsNumericRating(t *testing.T) {
	data := []byte(`{"content_id":"abc123","title":"Example","review":{"average":4.5}}`)
	item, err := ParseFanza(data)
	if err != nil || item.Item.MetadataJSON == "" {
		t.Fatalf("numeric rating: %v", err)
	}
}

func TestParseFanzaUsesSmallImageWhenLargeIsMissing(t *testing.T) {
	body := []byte(`{"content_id":"abc123","title":"Example","imageURL":{"small":"https://example.com/small.jpg"}}`)
	item, err := ParseFanza(body)
	if err != nil || item.Item.CoverImageURL != "https://example.com/small.jpg" {
		t.Fatalf("cover=%q err=%v", item.Item.CoverImageURL, err)
	}
}

func TestParseFanzaSupportsStringActressID(t *testing.T) {
	body := []byte(`{"content_id":"abc123","title":"Example","iteminfo":{"actress":[{"id":"42","name":"演员"}]}}`)
	item, err := ParseFanza(body)
	if err != nil || len(item.Cast) != 1 || item.Cast[0].FanzaID == nil || *item.Cast[0].FanzaID != 42 {
		t.Fatalf("cast=%+v err=%v", item.Cast, err)
	}
}
