package sources

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func TestBrowserRateLimitIncludesRetryDeadline(t *testing.T) {
	err := browserResponseError(&network.Response{Status: 429, Headers: network.Headers{"retry-after": "3600"}})
	var request *RequestError
	if !errors.As(err, &request) || request.Status != 429 || time.Until(request.RetryAt) < 59*time.Minute {
		t.Fatalf("浏览器限流时间未保留: %v", err)
	}
}

func TestBrowserProxyRejectsNonPublicAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.0.1", "::1", "fc00::1", "2001:2::10"} {
		if publicIP(net.ParseIP(address)) {
			t.Errorf("不应允许浏览器访问 %s", address)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("应允许公网地址")
	}
}

func TestLiveChromiumLaunch(t *testing.T) {
	if os.Getenv("LIBREFANZA_LIVE_BROWSER") == "" {
		t.Skip("需要显式启用浏览器检查")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-gpu", true))
	allocator, closeAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAllocator()
	browser, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	if err := chromedp.Do(browser, chromedp.Navigate("data:text/html,<html><body><script>document.body.textContent='rendered'</script></body></html>")); err != nil {
		t.Fatal(err)
	}
	value, err := chromedp.Run(browser, chromedp.Evaluate[string]("document.body.textContent"))
	if err != nil || value != "rendered" {
		t.Fatalf("浏览器未正确渲染页面: %q %v", value, err)
	}
}

func TestLiveChromiumProxy(t *testing.T) {
	if os.Getenv("LIBREFANZA_LIVE_BROWSER") == "" {
		t.Skip("需要显式启用浏览器检查")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	base, _ := url.Parse("https://javquery.cc/")
	provider := &httpProvider{base: base}
	proxy, closeProxy, err := provider.browserProxy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ProxyServer("http://"+proxy), chromedp.Flag("proxy-bypass-list", "<-loopback>"), chromedp.Flag("disable-quic", true))
	allocator, closeAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAllocator()
	browser, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	response, err := chromedp.Run(browser, chromedp.NavigateResponse(base.String()))
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || response.Status != 200 {
		t.Fatalf("无效响应: %+v", response)
	}
}

func TestBrowserSlotHonorsCancellation(t *testing.T) {
	release, err := acquireBrowser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := acquireBrowser(ctx); err == nil {
		t.Fatal("等待浏览器资源时应响应取消")
	}
}

func TestBrowserProxyRejectsCrossOriginAndLocalTargets(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	provider := &httpProvider{base: base}
	address, closeProxy, err := provider.browserProxy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	for _, target := range []string{"127.0.0.1:443", "example.com:80", "other.example:443"} {
		connection, err := net.Dial("tcp", address)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fmt.Fprintf(connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
		line, err := bufio.NewReader(connection).ReadString('\n')
		_ = connection.Close()
		if err != nil || !strings.Contains(line, " 403 ") {
			t.Fatalf("目标 %s 未被拒绝: %q %v", target, line, err)
		}
	}
}
