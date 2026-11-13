package torrentplay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/iplist"
)

func TestPublicPeerFilter(t *testing.T) {
	f := peerBlocklist{}
	for _, tc := range []struct {
		ip      string
		blocked bool
	}{{"127.0.0.1", true}, {"192.168.1.1", true}, {"8.8.8.8", false}, {"::1", true}} {
		_, blocked := f.Lookup(net.ParseIP(tc.ip))
		if blocked != tc.blocked {
			t.Fatalf("地址过滤错误: %s", tc.ip)
		}
	}
	var _ iplist.Ranger = f
}

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, errors.New("数据超时") }
func (failedReader) Seek(offset int64, whence int) (int64, error) {
	if whence == io.SeekEnd {
		return 1024, nil
	}
	return offset, nil
}

func TestEmptyStreamReturnsErrorBeforeHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/video", nil)
	r.Header.Set("Range", "bytes=0-255")
	serveVideo(w, r, "video.mp4", failedReader{})
	if w.Code != 504 || w.Header().Get("Content-Range") != "" {
		t.Fatalf("应报告缓冲失败: %d %v", w.Code, w.Header())
	}
}

func TestStreamHonorsByteRanges(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/video", nil)
	r.Header.Set("Range", "bytes=2-5")
	serveVideo(w, r, "video.mp4", bytes.NewReader([]byte("0123456789")))
	if w.Code != 206 || w.Body.String() != "2345" || w.Header().Get("Content-Range") != "bytes 2-5/10" {
		t.Fatalf("范围响应错误: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
}

func TestProbeFirstBytesRejectsShortAndAcceptsEnough(t *testing.T) {
	if err := probeFirstBytes(bytes.NewReader(make([]byte, 1024))); err == nil {
		t.Fatal("首段不足时必须重试其他候选")
	}
	if err := probeFirstBytes(bytes.NewReader(make([]byte, 32<<10))); err != nil {
		t.Fatalf("可用的首段应通过: %v", err)
	}
}

func TestCloseReleasesSessionAndCache(t *testing.T) {
	config := torrent.NewDefaultClientConfig()
	config.DataDir = t.TempDir()
	config.NoDHT = true
	config.DisableTrackers = true
	config.NoDefaultPortForwarding = true
	config.ListenPort = 0
	client, err := torrent.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	manager := NewManager()
	manager.slots <- struct{}{}
	manager.sessions["test"] = &Session{Client: client, Dir: dir, last: time.Now()}
	manager.Close("test")
	manager.Close("test")
	if manager.Session("test") != nil || len(manager.slots) != 0 {
		t.Fatal("会话或并发名额未释放")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("会话缓存未清理")
	}
}
func TestUnknownSessionIsRejected(t *testing.T) {
	m := NewManager()
	if m.Session("missing") != nil {
		t.Fatal("未知会话不应有效")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if _, err := m.Start(ctx, "invalid!"); err == nil {
		t.Fatal("无效番号应拒绝")
	}
}

func TestStartupBufferRanges(t *testing.T) {
	head, tail := startupRanges(100 << 20)
	if head.offset != 0 || head.length != 4<<20 || tail.offset != 98<<20 || tail.length != 2<<20 {
		t.Fatalf("缓冲范围错误: %+v %+v", head, tail)
	}
	head, tail = startupRanges(1 << 20)
	if head.length != 1<<20 || tail.offset != 0 || tail.length != 0 {
		t.Fatalf("小文件缓冲范围错误: %+v %+v", head, tail)
	}
}
