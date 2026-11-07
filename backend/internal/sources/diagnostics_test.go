package sources

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestSourceRequestReportsSafeNetworkReason(t *testing.T) {
	secret := "secret-token-123"
	provider := &httpProvider{client: &http.Client{Transport: failingTransport{err: &url.Error{
		Op: "Get", URL: "https://user:" + secret + "@example.com/?api_key=" + secret,
		Err: &net.DNSError{Err: "no such host", Name: secret, IsNotFound: true},
	}}}}
	_, _, err := provider.get(context.Background(), "https://example.com/?api_key="+secret, nil)
	if err == nil || !strings.Contains(err.Error(), "DNS") || strings.Contains(err.Error(), secret) {
		t.Fatalf("来源错误缺少安全的 DNS 原因：%v", err)
	}
}

func TestPostSourceRequestReportsSafeCertificateReason(t *testing.T) {
	provider := &httpProvider{base: &url.URL{Scheme: "https", Host: "example.com"}, client: &http.Client{Transport: failingTransport{err: x509.HostnameError{Host: "private.example"}}}}
	err := provider.postJSON(context.Background(), "/search", struct{}{}, new(any))
	var reason *RequestError
	if !errors.As(err, &reason) || reason.Reason != "tls" || strings.Contains(err.Error(), "private.example") {
		t.Fatalf("证书错误未安全分类：%v", err)
	}
}

func TestCrawlFailureIncludesPageAndPreservesCause(t *testing.T) {
	cause := errors.New("模拟失败")
	source := failingPageSource{err: cause}
	request := CrawlRequest{Source: "libredmm", Mode: "pages", StartPage: 4, EndPage: 4}
	err := RunCrawl(context.Background(), request, source, &stubSink{}, CrawlProgress{}, func(CrawlProgress) error { return nil })
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "4") {
		t.Fatalf("采集错误未包含页码或原始原因：%v", err)
	}
}

type failingPageSource struct{ err error }

func (s failingPageSource) Page(context.Context, int) ([]Parsed, error)      { return nil, s.err }
func (s failingPageSource) Search(context.Context, string) ([]Parsed, error) { return nil, s.err }
