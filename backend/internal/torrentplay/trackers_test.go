package torrentplay

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"
)

func TestFilterTrackersPinsPublicIPAndLimits(t *testing.T) {
	resolver := func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "bad.example" {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}
	items := []string{"udp://bad.example:6969/announce", "udp://good.example:6969/announce", "http://good.example:80/announce", "udp://8.8.8.8:6969/announce"}
	got := filterTrackers(context.Background(), items, 1, resolver)
	if len(got) != 1 {
		t.Fatalf("数量错误: %v", got)
	}
	parsed, _ := url.Parse(got[0])
	if parsed.Hostname() != "8.8.8.8" || strings.Contains(got[0], "bad") {
		t.Fatalf("地址无效: %v", got)
	}
}
