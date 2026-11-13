package torrentplay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/missinggo/v2/filecache"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/iplist"
	"github.com/anacrolix/torrent/storage"
)

const cacheCapacity = 512 << 20

var ErrBusy = errors.New("playback busy")
var ErrUnavailable = errors.New("torrent unavailable")

type peerBlocklist struct{}

func (peerBlocklist) Lookup(ip net.IP) (iplist.Range, bool) { return iplist.Range{}, !publicIP(ip) }
func (peerBlocklist) NumRanges() int                        { return 1 }

type Session struct {
	Client      *torrent.Client
	File        *torrent.File
	Dir         string
	Title       string
	Seeders     int
	Page        string
	mu          sync.Mutex
	last        time.Time
	created     time.Time
	closed      bool
	cancel      context.CancelFunc
	warmWorkers sync.WaitGroup
}
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	slots    chan struct{}
	search   *http.Client
}

func NewManager() *Manager {
	return &Manager{sessions: map[string]*Session{}, slots: make(chan struct{}, 2), search: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Scheme != "https" || (req.URL.Hostname() != "sukebei.nyaa.si" && req.URL.Hostname() != "raw.githubusercontent.com") {
			return errors.New("redirect rejected")
		}
		return nil
	}}}
}
func (m *Manager) Start(ctx context.Context, id string) (token string, err error) {
	if !regexp.MustCompile(`^[A-Za-z0-9]+(?:-[A-Za-z0-9]+)+$`).MatchString(id) {
		return "", ErrUnavailable
	}
	select {
	case m.slots <- struct{}{}:
	default:
		return "", ErrBusy
	}
	success := false
	defer func() {
		if !success {
			<-m.slots
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	results, err := Search(ctx, m.search, "https://sukebei.nyaa.si/", id)
	if err != nil || len(results) == 0 {
		return "", ErrUnavailable
	}
	candidates := results
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	for _, item := range candidates {
		if item.Seeders <= 0 {
			continue
		}
		parsed, _ := url.Parse(item.Magnet)
		vals := parsed.Query()
		extra := trackerList(ctx, m.search)
		trackerCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		trackers := filterTrackers(trackerCtx, vals["tr"], 5, net.DefaultResolver.LookupIPAddr)
		stop()
		seen := map[string]bool{}
		clean := url.Values{"xt": []string{vals.Get("xt")}}
		for _, v := range append(trackers, extra...) {
			if !seen[v] {
				seen[v] = true
				clean.Add("tr", v)
			}
		}
		dir, e := os.MkdirTemp("", "librefanza-torrent-")
		if e != nil {
			return "", e
		}
		cache, e := filecache.NewCache(filepath.Join(dir, "pieces"))
		if e != nil {
			os.RemoveAll(dir)
			continue
		}
		cache.SetCapacity(cacheCapacity)
		capacity := func() (int64, bool) { return cacheCapacity, true }
		cfg := torrent.NewDefaultClientConfig()
		cfg.Slogger = slog.New(quietTransientReader{slog.Default().Handler()})
		cfg.DefaultStorage = storage.NewResourcePiecesOpts(cache.AsResourceProvider(), storage.ResourcePiecesOpts{Capacity: &capacity})
		cfg.ListenPort = 0
		cfg.NoUpload = true
		cfg.NoDefaultPortForwarding = true
		cfg.DisableWebtorrent = true
		cfg.DisableWebseeds = true
		cfg.IPBlocklist = peerBlocklist{}
		cfg.EstablishedConnsPerTorrent = 50
		cfg.MaxUnverifiedBytes = 32 << 20
		client, e := torrent.NewClient(cfg)
		if e != nil {
			os.RemoveAll(dir)
			continue
		}
		tor, e := client.AddMagnet("magnet:?" + clean.Encode())
		if e != nil {
			client.Close()
			os.RemoveAll(dir)
			continue
		}
		wait := time.NewTimer(18 * time.Second)
		select {
		case <-tor.GotInfo():
			if !wait.Stop() {
				<-wait.C
			}
		case <-wait.C:
			client.Close()
			os.RemoveAll(dir)
			continue
		case <-ctx.Done():
			if !wait.Stop() {
				select {
				case <-wait.C:
				default:
				}
			}
			client.Close()
			os.RemoveAll(dir)
			return "", ctx.Err()
		}
		files := tor.Files()
		entries := make([]videoFile, len(files))
		for i, f := range files {
			entries[i] = videoFile{f.DisplayPath(), f.Length()}
		}
		idx := selectVideo(entries)
		if idx < 0 {
			client.Close()
			os.RemoveAll(dir)
			continue
		}
		probeCtx, stopProbe := context.WithTimeout(ctx, 15*time.Second)
		probe := files[idx].NewReader()
		probe.SetContext(probeCtx)
		probe.SetReadahead(8 << 20)
		probe.SetResponsive()
		probeErr := probeFirstBytes(probe)
		probe.Close()
		stopProbe()
		if probeErr != nil {
			client.Close()
			os.RemoveAll(dir)
			continue
		}
		var b [24]byte
		if _, e = rand.Read(b[:]); e != nil {
			client.Close()
			os.RemoveAll(dir)
			return "", e
		}
		token = hex.EncodeToString(b[:])
		s := &Session{Client: client, File: files[idx], Dir: dir, Title: item.Title, Seeders: item.Seeders, Page: item.Page, last: time.Now(), created: time.Now()}
		warmCtx, warmCancel := context.WithCancel(context.Background())
		s.cancel = warmCancel
		prefetchFile(warmCtx, s.File, &s.warmWorkers)
		m.mu.Lock()
		m.sessions[token] = s
		m.mu.Unlock()
		success = true
		go m.expire(token, s)
		return token, nil
	}
	return "", ErrUnavailable
}
func (m *Manager) Session(token string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[token]
	if s != nil {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil
		} else {
			s.last = time.Now()
		}
		s.mu.Unlock()
	}
	return s
}
func (m *Manager) expire(token string, s *Session) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		expired := s.closed || time.Since(s.last) > 5*time.Minute || time.Since(s.created) > 4*time.Hour
		s.mu.Unlock()
		if expired {
			m.Close(token)
			return
		}
	}
}
func (m *Manager) Close(token string) {
	m.mu.Lock()
	s := m.sessions[token]
	if s != nil {
		delete(m.sessions, token)
	}
	m.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.Client.Close()
	s.warmWorkers.Wait()
	os.RemoveAll(s.Dir)
	<-m.slots
}

type activeReader struct {
	reader  torrent.Reader
	ctx     context.Context
	session *Session
}

func (r activeReader) Read(p []byte) (int, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 45*time.Second)
	defer cancel()
	r.reader.SetContext(ctx)
	n, err := r.reader.Read(p)
	if n > 0 {
		r.session.mu.Lock()
		r.session.last = time.Now()
		r.session.mu.Unlock()
	}
	return n, err
}

func (r activeReader) Seek(offset int64, whence int) (int64, error) {
	return r.reader.Seek(offset, whence)
}

func (s *Session) Serve(w http.ResponseWriter, r *http.Request) {
	reader := s.File.NewReader()
	defer reader.Close()
	reader.SetContext(r.Context())
	reader.SetReadahead(32 << 20)
	reader.SetResponsive()
	ext := strings.ToLower(filepath.Ext(s.File.DisplayPath()))
	if ext == ".webm" {
		w.Header().Set("Content-Type", "video/webm")
	} else {
		w.Header().Set("Content-Type", "video/mp4")
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	serveVideo(w, r, "video"+ext, activeReader{reader: reader, ctx: r.Context(), session: s})
}
