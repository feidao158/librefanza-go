package sources

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type addressLookup func(context.Context, string) ([]net.IPAddr, error)

func ResolvePublicAddresses(ctx context.Context, host string) ([]net.IPAddr, error) {
	return resolvePublicAddresses(ctx, host, net.DefaultResolver.LookupIPAddr, lookupPublicDNS)
}

func resolvePublicAddresses(ctx context.Context, host string, lookup, fallback addressLookup) ([]net.IPAddr, error) {
	if host == "" || net.ParseIP(host) != nil || strings.ContainsAny(host, "/:@[]%") {
		return nil, errors.New("invalid public host")
	}
	addresses, err := lookup(ctx, host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("public host unavailable")
	}
	_, fakeNetwork, _ := net.ParseCIDR("198.18.0.0/15")
	_, fakeNetwork6, _ := net.ParseCIDR("2001:2::/48")
	var public []net.IPAddr
	hasFake := false
	for _, address := range addresses {
		if fakeNetwork.Contains(address.IP) || fakeNetwork6.Contains(address.IP) {
			hasFake = true
			continue
		}
		if !publicIP(address.IP) {
			return nil, errors.New("public host denied")
		}
		public = append(public, address)
	}
	if len(public) == 0 && hasFake {
		addresses, err = fallback(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("public host unavailable")
		}
		for _, address := range addresses {
			if !publicIP(address.IP) {
				return nil, errors.New("public host denied")
			}
			public = append(public, address)
		}
	}
	return public, nil
}

func lookupPublicDNS(ctx context.Context, host string) ([]net.IPAddr, error) {
	endpoint := &url.URL{Scheme: "https", Host: "cloudflare-dns.com", Path: "/dns-query", RawQuery: url.Values{"name": {host}, "type": {"A"}}.Encode()}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/dns-json")
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("public DNS unavailable")
	}
	var result struct {
		Status int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil || result.Status != 0 {
		return nil, errors.New("invalid public DNS response")
	}
	var addresses []net.IPAddr
	for _, answer := range result.Answer {
		if answer.Type == 1 {
			ip := net.ParseIP(answer.Data)
			if ip == nil {
				return nil, errors.New("invalid public DNS response")
			}
			addresses = append(addresses, net.IPAddr{IP: ip})
		}
	}
	return addresses, nil
}
