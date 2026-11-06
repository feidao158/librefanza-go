package images

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestFetchSODImageSetsRefererAndRejectsOtherHosts(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("Referer") != "https://ec.sod.co.jp/prime/videos/?id=ABC-123" {
			t.Fatalf("unexpected referer %q", request.Header.Get("Referer"))
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"image/jpeg"}}, Body: io.NopCloser(strings.NewReader("\xff\xd8\xff\xdbtest")), Request: request}, nil
	})}
	body, contentType, err := FetchSOD(context.Background(), client, "https://dy43ylo5q3vt8.cloudfront.net/image.jpg", "https://ec.sod.co.jp/prime/videos/?id=ABC-123")
	if err != nil || len(body) == 0 || contentType != "image/jpeg" {
		t.Fatalf("body=%q type=%q err=%v", body, contentType, err)
	}
	for _, value := range []string{"http://127.0.0.1/image", "https://attacker.example/image", "https://dy43ylo5q3vt8.cloudfront.net:444/image"} {
		if AllowedSODURL(value) {
			t.Fatalf("accepted %q", value)
		}
	}
}
