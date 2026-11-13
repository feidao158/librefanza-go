package torrentplay

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

type bufferRange struct{ offset, length int64 }

func probeFirstBytes(reader io.Reader) error {
	_, err := io.CopyN(io.Discard, reader, 32<<10)
	return err
}

func startupRanges(size int64) (bufferRange, bufferRange) {
	if size <= 0 {
		return bufferRange{}, bufferRange{}
	}
	head := min(size, int64(4<<20))
	tail := min(size-head, int64(2<<20))
	if tail == 0 {
		return bufferRange{0, head}, bufferRange{}
	}
	return bufferRange{0, head}, bufferRange{size - tail, tail}
}

// 在后台提前读取文件开头及尾部索引，关闭会话时自动取消。
func prefetchFile(ctx context.Context, file *torrent.File, workers *sync.WaitGroup) {
	head, tail := startupRanges(file.Length())
	for _, part := range []bufferRange{head, tail} {
		if part.length == 0 {
			continue
		}
		workers.Add(1)
		go func(part bufferRange) {
			defer workers.Done()
			readCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			r := file.NewReader()
			defer r.Close()
			r.SetContext(readCtx)
			r.SetResponsive()
			if part.offset == 0 {
				r.SetReadahead(32 << 20)
			} else {
				r.SetReadahead(2 << 20)
			}
			if _, err := r.Seek(part.offset, io.SeekStart); err == nil {
				_, _ = io.CopyN(io.Discard, r, part.length)
			}
		}(part)
	}
}

type delayedResponse struct {
	http.ResponseWriter
	status  int
	written bool
}

func (d *delayedResponse) WriteHeader(status int) {
	if d.status == 0 {
		d.status = status
	}
}
func (d *delayedResponse) Write(data []byte) (int, error) {
	if !d.written {
		d.written = true
		if d.status == 0 {
			d.status = http.StatusOK
		}
		d.ResponseWriter.WriteHeader(d.status)
	}
	return d.ResponseWriter.Write(data)
}

// 首块无法读取时先返回错误，避免将空响应误报为正常的视频范围。
func serveVideo(w http.ResponseWriter, r *http.Request, name string, reader io.ReadSeeker) {
	if r.Method == http.MethodHead {
		http.ServeContent(w, r, name, time.Time{}, reader)
		return
	}
	pending := &delayedResponse{ResponseWriter: w}
	http.ServeContent(pending, r, name, time.Time{}, reader)
	if pending.written {
		return
	}
	if pending.status >= 400 {
		w.WriteHeader(pending.status)
		return
	}
	for _, name := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Content-Type"} {
		w.Header().Del(name)
	}
	http.Error(w, "video buffering timed out", http.StatusGatewayTimeout)
}
