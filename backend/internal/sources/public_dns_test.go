package sources

import (
	"context"
	"net"
	"os"
	"testing"
)

func TestResolvePublicAddressesOnlyFallsBackForFakeIP(t *testing.T) {
	lookup := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "private.example" {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.10")}, {IP: net.ParseIP("2001:2::10")}}, nil
	}
	fallbackCalls := 0
	fallback := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		fallbackCalls++
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	addresses, err := resolvePublicAddresses(context.Background(), "source.example", lookup, fallback)
	if err != nil || len(addresses) != 1 || !addresses[0].IP.Equal(net.ParseIP("8.8.8.8")) || fallbackCalls != 1 {
		t.Fatalf("Fake-IP 回退失败: %v %d %v", addresses, fallbackCalls, err)
	}
	if _, err := resolvePublicAddresses(context.Background(), "private.example", lookup, fallback); err == nil || fallbackCalls != 1 {
		t.Fatal("内网地址不应触发公共 DNS 回退")
	}
	mixed := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.10")}, {IP: net.ParseIP("2606:4700:4700::1111")}}, nil
	}
	addresses, err = resolvePublicAddresses(context.Background(), "source.example", mixed, fallback)
	if err != nil || len(addresses) != 1 || !addresses[0].IP.Equal(net.ParseIP("2606:4700:4700::1111")) || fallbackCalls != 1 {
		t.Fatalf("公网与 Fake-IP 混合解析错误: %v %d %v", addresses, fallbackCalls, err)
	}
	badFallback := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
	}
	if _, err := resolvePublicAddresses(context.Background(), "source.example", lookup, badFallback); err == nil {
		t.Fatal("公共 DNS 返回内网地址时必须拒绝")
	}
}

func TestLivePublicDNS(t *testing.T) {
	if os.Getenv("LIBREFANZA_LIVE_DNS") == "" {
		t.Skip("需要显式启用在线 DNS 检查")
	}
	addresses, err := ResolvePublicAddresses(context.Background(), "javquery.cc")
	if err != nil || len(addresses) == 0 {
		t.Fatalf("公共 DNS 解析失败: %v", err)
	}
	t.Logf("已解析到 %d 个公网地址", len(addresses))
}
