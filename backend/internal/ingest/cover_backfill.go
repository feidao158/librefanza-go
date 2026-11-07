package ingest

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/libredmm/librefanza/backend/internal/cache"
	"github.com/libredmm/librefanza/backend/internal/catalog"
	"github.com/libredmm/librefanza/backend/internal/sources"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
)

func improvedCover(old, candidate image.Config, hasOld bool) bool {
	if candidate.Width <= 0 || candidate.Height <= 0 || candidate.Width > 30000 || candidate.Height > 30000 {
		return false
	}
	area := int64(candidate.Width) * int64(candidate.Height)
	if hasOld {
		return old.Width > 0 && old.Height > 0 && area > int64(old.Width)*int64(old.Height)
	}
	return area >= 160000 && (candidate.Width >= 400 || candidate.Height >= 400)
}

func (s *Service) ImproveCover(ctx context.Context, settings sources.ClientSettings, item catalog.SourceItem) error {
	providers, err := sources.NewProviders(settings)
	if err != nil {
		return err
	}
	var candidates []sources.Parsed
	for _, provider := range providers {
		named, ok := provider.(interface{ SourceName() string })
		if !ok || named.SourceName() != item.Source {
			continue
		}
		candidates, err = provider.Search(ctx, item.NormalizedID)
		if err != nil {
			return err
		}
		break
	}
	failed := false
	for _, parsed := range candidates {
		candidate := parsed.Item.CoverImageURL
		if !sameCoverItem(item, parsed.Item) {
			continue
		}
		if candidate == "" || candidate == item.CoverImageURL {
			continue
		}
		newSize, err := readImageSize(ctx, settings.RequestLimiter, settings.ProxyURL, item.Source, candidate, item.SourceURL)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failed = true
			continue
		}
		oldSize, hasOld := image.Config{}, item.CoverImageURL != ""
		if hasOld {
			oldSize, err = readImageSize(ctx, settings.RequestLimiter, settings.ProxyURL, item.Source, item.CoverImageURL, item.SourceURL)
			if err != nil {
				return err
			}
		}
		if !improvedCover(oldSize, newSize, hasOld) {
			continue
		}
		return s.saveImprovedCover(ctx, item, candidate)
	}
	if failed {
		return errors.New("image unavailable")
	}
	return nil
}

func sameCoverItem(old, current catalog.SourceItem) bool {
	return current.NormalizedID == old.NormalizedID && (old.ExternalID == "" || current.ExternalID == old.ExternalID)
}

func (s *Service) saveImprovedCover(ctx context.Context, item catalog.SourceItem, candidate string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Model(&catalog.SourceItem{}).Where("id = ?", item.ID)
		if item.CoverImageURL == "" {
			query = query.Where("(cover_image_url = ? OR cover_image_url IS NULL)", "")
		} else {
			query = query.Where("cover_image_url = ?", item.CoverImageURL)
		}
		result := query.Updates(map[string]any{"cover_image_url": candidate, "cover_is_improved": true, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return cache.BumpVersion(tx)
	})
}

func readImageSize(ctx context.Context, limiter sources.RequestLimiter, proxyAddress, source, address, sourceURL string) (image.Config, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" || net.ParseIP(parsed.Hostname()) != nil || (parsed.Port() != "" && parsed.Port() != "443") {
		return image.Config{}, errors.New("invalid image URL")
	}
	if limiter != nil {
		if err := limiter.Wait(ctx, source); err != nil {
			return image.Config{}, err
		}
	}
	var proxy *url.URL
	if proxyAddress != "" {
		proxy, err = url.Parse(proxyAddress)
		if err != nil || proxy.Hostname() == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
			return image.Config{}, errors.New("invalid proxy URL")
		}
	}
	transport := &http.Transport{DialContext: imageDialer(proxy, sources.ResolvePublicAddresses), TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return image.Config{}, errors.New("invalid image URL")
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	if ref, err := url.Parse(sourceURL); err == nil && ref.User == nil && (ref.Scheme == "https" || ref.Scheme == "http") && ref.Hostname() != "" {
		request.Header.Set("Referer", ref.Scheme+"://"+ref.Host+"/")
	}
	response, err := client.Do(request)
	if err != nil {
		return image.Config{}, errors.New("image unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return image.Config{}, errors.New("image unavailable")
	}
	config, _, err := image.DecodeConfig(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return image.Config{}, errors.New("invalid image")
	}
	return config, nil
}

func imageDialer(proxy *url.URL, resolve func(context.Context, string) ([]net.IPAddr, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port != "443" || net.ParseIP(host) != nil {
			return nil, errors.New("invalid image host")
		}
		answers, err := resolve(ctx, host)
		if err != nil || len(answers) == 0 {
			return nil, errors.New("image host unavailable")
		}
		for _, answer := range answers {
			if !publicImageIP(answer.IP) {
				return nil, errors.New("image host denied")
			}
		}
		target := net.JoinHostPort(answers[0].IP.String(), port)
		dialer := &net.Dialer{Timeout: 10 * time.Second}
		if proxy == nil {
			return dialer.DialContext(ctx, network, target)
		}
		proxyPort := proxy.Port()
		if proxyPort == "" {
			if proxy.Scheme == "https" {
				proxyPort = "443"
			} else {
				proxyPort = "80"
			}
		}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(proxy.Hostname(), proxyPort))
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
			return nil, errors.New("image proxy connection failed")
		}
		_ = conn.SetDeadline(time.Time{})
		return conn, nil
	}
}

func publicImageIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	for _, text := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:2::/48", "2001:db8::/32"} {
		_, block, _ := net.ParseCIDR(text)
		if block.Contains(ip) {
			return false
		}
	}
	return !strings.Contains(ip.String(), "%")
}
