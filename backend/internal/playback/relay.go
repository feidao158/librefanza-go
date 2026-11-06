package playback

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

var blockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func publicAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedNetworks {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func safeClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" {
			return nil, errors.New("invalid playback address")
		}
		ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil || len(ips) == 0 {
			return nil, errors.New("playback host unavailable")
		}
		for _, ip := range ips {
			if !publicAddress(ip) {
				return nil, errors.New("invalid playback address")
			}
		}
		var last error
		for _, ip := range ips {
			connection, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return connection, nil
			}
			last = err
		}
		return nil, last
	}
	return &http.Client{Transport: transport, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 4 || !validTarget(request.URL.String()) {
			return errors.New("invalid playback redirect")
		}
		return nil
	}}
}

func (s *Service) Relay(w http.ResponseWriter, request *http.Request, token string) {
	address, referer, err := s.Verify(token)
	if err != nil {
		http.Error(w, "invalid playback token", http.StatusForbidden)
		return
	}
	upstream, err := http.NewRequestWithContext(request.Context(), http.MethodGet, address, nil)
	if err != nil {
		http.Error(w, "playback unavailable", http.StatusBadGateway)
		return
	}
	upstream.Header.Set("Referer", referer)
	if value := request.Header.Get("Range"); value != "" && regexp.MustCompile(`^bytes=\d*-\d*$`).MatchString(value) {
		upstream.Header.Set("Range", value)
	}
	response, err := s.client.Do(upstream)
	if err != nil {
		http.Error(w, "playback unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		http.Error(w, "playback unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if strings.Contains(contentType, "mpegurl") || strings.HasSuffix(strings.ToLower(response.Request.URL.Path), ".m3u8") {
		body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
		if err != nil || len(body) > 4<<20 {
			http.Error(w, "invalid playback playlist", http.StatusBadGateway)
			return
		}
		body, err = s.RewritePlaylist(body, response.Request.URL.String(), referer, time.Now().Add(4*time.Hour))
		if err != nil {
			http.Error(w, "invalid playback playlist", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = w.Write(body)
		return
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}
