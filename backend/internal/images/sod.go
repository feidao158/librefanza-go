package images

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func AllowedSODURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" && parsed.Port() != "443" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "dy43ylo5q3vt8.cloudfront.net" || host == "sod.co.jp" || strings.HasSuffix(host, ".sod.co.jp")
}

func FetchSOD(ctx context.Context, client *http.Client, imageURL, referer string) ([]byte, string, error) {
	if !AllowedSODURL(imageURL) {
		return nil, "", errors.New("unsupported image host")
	}
	ref, err := url.Parse(referer)
	if err != nil || (ref.Scheme != "https" && ref.Scheme != "http") || !strings.EqualFold(ref.Hostname(), "ec.sod.co.jp") {
		return nil, "", errors.New("invalid image referer")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Referer", referer)
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", errors.New("image request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", errors.New("image unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (10<<20)+1))
	if err != nil || len(body) == 0 || len(body) > 10<<20 {
		return nil, "", errors.New("invalid image size")
	}
	contentType := http.DetectContentType(body)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		return nil, "", errors.New("unsupported image format")
	}
	return body, contentType, nil
}
