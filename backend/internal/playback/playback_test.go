package playback

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestJableStreamExtraction(t *testing.T) {
	value, err := ExtractJableStream([]byte(`<script>var hlsUrl = "https://cdn.example.com/movie/index.m3u8?token=123";</script>`))
	if err != nil || value != "https://cdn.example.com/movie/index.m3u8?token=123" {
		t.Fatalf("地址 = %s，错误 = %v", value, err)
	}
	if _, err := ExtractJableStream([]byte(`<script>var hlsUrl = "http://127.0.0.1/index.m3u8";</script>`)); err == nil {
		t.Fatal("不应允许本地地址")
	}
}

func TestRelayTokenRejectsModificationAndExpiry(t *testing.T) {
	service := NewService()
	token, err := service.Sign("https://cdn.example.com/index.m3u8", "https://jable.tv/videos/abc-123/", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, "cdn.example.com") {
		t.Fatal("令牌不应包含可读的源地址")
	}
	if _, _, err := service.Verify(token + "x"); err == nil {
		t.Fatal("篡改的令牌不应通过验证")
	}
	expired, _ := service.Sign("https://cdn.example.com/index.m3u8", "https://jable.tv/videos/abc-123/", time.Now().Add(-time.Minute))
	if _, _, err := service.Verify(expired); err == nil {
		t.Fatal("过期的令牌不应通过验证")
	}
}

func TestPublicAddressRejectsPrivateNetworks(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "::ffff:127.0.0.1"} {
		if publicAddress(netip.MustParseAddr(value)) {
			t.Errorf("不应允许地址 %s", value)
		}
	}
	if !publicAddress(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("公网地址未通过校验")
	}
}

func TestRewritePlaylistSignsSegmentsAndKeys(t *testing.T) {
	service := NewService()
	input := "#EXTM3U\n#EXT-X-TARGETDURATION:10\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXTINF:8,\nsegment.ts\n#EXT-X-ENDLIST\n"
	result, err := service.RewritePlaylist([]byte(input), "https://cdn.example.com/path/index.m3u8", "https://jable.tv/videos/abc-123/", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(result), "segment.ts") || strings.Contains(string(result), "key.bin") || strings.Count(string(result), "/api/v1/playback/relay/") != 2 {
		t.Fatalf("播放列表 = %s", result)
	}
}
