package sources

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

var browserSlots = make(chan struct{}, 1)

func acquireBrowser(ctx context.Context) (func(), error) {
	select {
	case browserSlots <- struct{}{}:
		return func() { <-browserSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func publicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	for _, value := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:2::/48", "2001:db8::/32"} {
		_, block, _ := net.ParseCIDR(value)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func browserResponseError(response *network.Response) error {
	if response == nil {
		return errors.New("invalid source page")
	}
	err := &RequestError{Reason: "http", Status: int(response.Status)}
	if response.Status == http.StatusTooManyRequests {
		var header string
		for key, value := range response.Headers {
			if strings.EqualFold(key, "retry-after") {
				header = fmt.Sprint(value)
				break
			}
		}
		now := time.Now()
		err.RetryAt = now.Add(retryAfter(header, now))
	}
	return err
}

func (p *httpProvider) browserError(ctx context.Context, response *network.Response) error {
	err := browserResponseError(response)
	var request *RequestError
	if errors.As(err, &request) && !request.RetryAt.IsZero() {
		if p.settings.RequestLimiter != nil {
			if pauseErr := p.settings.RequestLimiter.Pause(ctx, p.kind, request.RetryAt); pauseErr != nil {
				return pauseErr
			}
		} else {
			p.gate.deferUntil(request.RetryAt)
		}
	}
	return err
}

func (p *httpProvider) browserProxy(ctx context.Context) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil || !strings.EqualFold(host, p.base.Hostname()) || port != "443" || net.ParseIP(host) != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		addresses, err := ResolvePublicAddresses(ctx, host)
		if err != nil || len(addresses) == 0 {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		upstream, err := p.dialBrowserTarget(ctx, net.JoinHostPort(addresses[0].IP.String(), port))
		if err != nil {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			upstream.Close()
			return
		}
		client, buffered, err := hijacker.Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		defer client.Close()
		defer upstream.Close()
		if _, err = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		if err = buffered.Flush(); err != nil {
			return
		}
		go func() { _, _ = io.Copy(upstream, buffered); _ = upstream.Close() }()
		_, _ = io.Copy(client, upstream)
	})}
	go func() { _ = server.Serve(listener) }()
	return listener.Addr().String(), func() { _ = server.Close(); _ = listener.Close() }, nil
}

func (p *httpProvider) dialBrowserTarget(ctx context.Context, target string) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if p.settings.ProxyURL == "" {
		return dialer.DialContext(ctx, "tcp", target)
	}
	proxy, err := url.Parse(p.settings.ProxyURL)
	if err != nil || proxy.Hostname() == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
		return nil, errors.New("invalid proxy URL")
	}
	port := proxy.Port()
	if port == "" {
		if proxy.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxy.Hostname(), port))
	if err != nil {
		return nil, err
	}
	if proxy.Scheme == "https" {
		secure := tls.Client(conn, &tls.Config{ServerName: proxy.Hostname(), MinVersion: tls.VersionTLS12})
		if err := secure.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		conn = secure
	}
	request := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: make(http.Header)}
	if proxy.User != nil {
		password, _ := proxy.User.Password()
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(proxy.User.Username()+":"+password)))
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := request.Write(conn); err != nil {
		conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, request)
	if err != nil || response.StatusCode != http.StatusOK || reader.Buffered() != 0 {
		conn.Close()
		return nil, errors.New("source proxy connection failed")
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func (p *httpProvider) browser(ctx context.Context, target, search string) ([]byte, *url.URL, error) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, p.base.Host) || parsed.User != nil {
		return nil, nil, errors.New("invalid source URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	release, err := acquireBrowser(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer release()
	if p.settings.RequestLimiter != nil {
		err = p.settings.RequestLimiter.Wait(ctx, p.kind)
	} else {
		err = p.gate.wait(ctx)
	}
	if err != nil {
		return nil, nil, err
	}
	proxy, closeProxy, err := p.browserProxy(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer closeProxy()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ProxyServer("http://"+proxy), chromedp.Flag("proxy-bypass-list", "<-loopback>"), chromedp.Flag("disable-quic", true), chromedp.Flag("disable-background-networking", true), chromedp.Flag("force-webrtc-ip-handling-policy", "disable_non_proxied_udp"), chromedp.Flag("blink-settings", "imagesEnabled=false"), chromedp.Flag("disable-gpu", true), chromedp.Flag("disable-software-rasterizer", true), chromedp.Flag("autoplay-policy", "user-gesture-required"))
	allocator, closeAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAllocator()
	browser, closeBrowser := chromedp.NewContext(allocator)
	defer closeBrowser()
	actions := []chromedp.Action[chromedp.Void]{}
	if cookie := p.sourceCookie(); cookie != "" {
		actions = append(actions, chromedp.Func(func(ctx context.Context, target *chromedp.Target) error {
			_, err := cdp.Call(ctx, target, network.SetExtraHTTPHeaders, network.SetExtraHTTPHeadersParams{Headers: network.Headers{"Cookie": cookie}})
			return err
		}))
	}
	if err := chromedp.Do(browser, actions...); err != nil {
		return nil, nil, requestError(err)
	}
	response, err := chromedp.Run(browser, chromedp.NavigateResponse(target))
	if err != nil {
		return nil, nil, requestError(err)
	}
	if response == nil || response.Status < 200 || response.Status >= 300 {
		return nil, nil, p.browserError(ctx, response)
	}
	if search != "" {
		encoded, _ := json.Marshal(search)
		if p.kind == "javbooks" {
			if p.settings.RequestLimiter != nil {
				err = p.settings.RequestLimiter.Wait(ctx, p.kind)
			} else {
				err = p.gate.wait(ctx)
			}
			if err != nil {
				return nil, nil, err
			}
			script := fmt.Sprintf(`(async () => { const r = await fetch('/serch_censored.htm', {method:'POST', headers:{'Content-Type':'application/x-www-form-urlencoded'}, body:new URLSearchParams({skey:%s}), redirect:'error'}); const retry=r.headers.get('retry-after')||''; if (!r.body) return {status:r.status,body:'',retry}; const reader=r.body.getReader(); let size=0, chunks=[]; while(true){const v=await reader.read(); if(v.done) break; size+=v.value.byteLength; if(size>8388608){await reader.cancel();return {status:r.status,body:'',retry}} chunks.push(v.value)} const all=new Uint8Array(size); let n=0; for(const c of chunks){all.set(c,n);n+=c.length} return {status:r.status,body:new TextDecoder().decode(all),retry}; })()`, encoded)
			result, err := chromedp.Run(browser, chromedp.Evaluate[struct {
				Status int    `json:"status"`
				Body   string `json:"body"`
				Retry  string `json:"retry"`
			}](script, chromedp.EvalAwaitPromise))
			if err != nil {
				return nil, nil, requestError(err)
			}
			if result.Status < 200 || result.Status >= 300 {
				return nil, nil, p.browserError(ctx, &network.Response{Status: int64(result.Status), Headers: network.Headers{"Retry-After": result.Retry}})
			}
			if result.Body == "" || len(result.Body) > 8<<20 {
				return nil, nil, errors.New("source response too large or unreadable")
			}
			return []byte(result.Body), p.base.ResolveReference(&url.URL{Path: "/serch_censored.htm"}), nil
		}
		script := fmt.Sprintf(`(() => {const f=document.querySelector('form.ym-search'); if(!f) return ''; const u=new URL(f.action,location.href); for(const e of f.elements) if(e.name && e.value) u.searchParams.set(e.name,e.value); u.searchParams.set('s',%s); return u.href})()`, encoded)
		searchURL, err := chromedp.Run(browser, chromedp.Evaluate[string](script))
		if err != nil || searchURL == "" {
			return nil, nil, errors.New("invalid source page")
		}
		u, err := url.Parse(searchURL)
		if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, p.base.Host) || u.Path != "/search.html" {
			return nil, nil, errors.New("invalid source page")
		}
		if p.settings.RequestLimiter != nil {
			err = p.settings.RequestLimiter.Wait(ctx, p.kind)
		} else {
			err = p.gate.wait(ctx)
		}
		if err != nil {
			return nil, nil, err
		}
		response, err := chromedp.Run(browser, chromedp.NavigateResponse(searchURL))
		if err != nil {
			return nil, nil, requestError(err)
		}
		if response == nil || response.Status < 200 || response.Status >= 300 {
			return nil, nil, p.browserError(ctx, response)
		}
	}
	length, err := chromedp.Run(browser, chromedp.Evaluate[int](`document.documentElement.outerHTML.length`))
	if err != nil {
		return nil, nil, requestError(err)
	}
	if length > 8<<20 {
		return nil, nil, errors.New("source response too large or unreadable")
	}
	html, err := chromedp.Run(browser, chromedp.OuterHTML("html"))
	if err != nil {
		return nil, nil, requestError(err)
	}
	current, err := chromedp.Run(browser, chromedp.Location())
	if err != nil {
		return nil, nil, requestError(err)
	}
	final, err := url.Parse(current)
	if err != nil || final.Scheme != "https" || !strings.EqualFold(final.Host, p.base.Host) || len(html) > 8<<20 {
		return nil, nil, errors.New("invalid source page")
	}
	return []byte(html), final, nil
}

func (p *httpProvider) renderPage(ctx context.Context, address string) ([]byte, error) {
	body, _, err := p.fetchBrowser(ctx, address, "")
	return body, err
}

func (p *httpProvider) searchRendered(ctx context.Context, id string) ([]byte, *url.URL, error) {
	return p.fetchBrowser(ctx, p.base.String(), id)
}
