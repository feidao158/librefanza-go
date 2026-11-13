package torrentplay

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var trackerCache struct {
	sync.Mutex
	expires time.Time
	values  []string
}
var forbiddenPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"),
}

func publicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range forbiddenPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

type lookupFunc func(context.Context, string) ([]net.IPAddr, error)

func filterTrackers(ctx context.Context, values []string, limit int, lookup lookupFunc) []string {
	result := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, v := range values {
		if len(result) >= limit || ctx.Err() != nil {
			break
		}
		u, err := url.Parse(strings.TrimSpace(v))
		if err != nil || u.Scheme != "udp" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Hostname() == "" || u.Path != "/announce" {
			continue
		}
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		ips, err := lookup(ctx, u.Hostname())
		if err != nil || len(ips) == 0 {
			continue
		}
		valid := true
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		u.Host = net.JoinHostPort(ips[0].IP.String(), strconv.Itoa(port))
		addr := u.String()
		if seen[addr] {
			continue
		}
		seen[addr] = true
		result = append(result, addr)
	}
	return result
}

// 获取并缓存公开 Tracker，固定解析后的公网地址。
func trackerList(ctx context.Context, client *http.Client) []string {
	trackerCache.Lock()
	defer trackerCache.Unlock()
	if time.Now().Before(trackerCache.expires) {
		return append([]string(nil), trackerCache.values...)
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/adysec/tracker/main/trackers_best_udp.txt", nil)
	if err == nil {
		response, e := client.Do(req)
		if e == nil {
			if response.StatusCode == http.StatusOK {
				data, e := io.ReadAll(io.LimitReader(response.Body, 128<<10))
				if e == nil {
					trackerCache.values = filterTrackers(ctx, strings.Split(string(data), "\n"), 12, net.DefaultResolver.LookupIPAddr)
				}
			}
			response.Body.Close()
		}
	}
	trackerCache.expires = time.Now().Add(6 * time.Hour)
	return append([]string(nil), trackerCache.values...)
}
