package playback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Eyevinn/hls-m3u8/m3u8"
)

var streamPattern = regexp.MustCompile(`(?i)\b(?:var|let|const)\s+hlsUrl\s*=\s*["']([^"']+)["']`)

type Service struct {
	secret [32]byte
	client *http.Client
}

type ticket struct {
	URL     string `json:"u"`
	Referer string `json:"r"`
	Expires int64  `json:"e"`
}

func NewService() *Service {
	service := &Service{client: safeClient()}
	if _, err := rand.Read(service.secret[:]); err != nil {
		panic("无法初始化播放服务")
	}
	return service
}

func validTarget(value string) bool {
	address, err := url.Parse(value)
	if err != nil || address.Scheme != "https" || address.Host == "" || address.User != nil || address.Fragment != "" {
		return false
	}
	host := strings.ToLower(address.Hostname())
	return host != "localhost" && !strings.HasSuffix(host, ".localhost") && net.ParseIP(host) == nil && (address.Port() == "" || address.Port() == "443")
}

func ExtractJableStream(body []byte) (string, error) {
	if len(body) == 0 || bytes.Contains(bytes.ToLower(body), []byte("challenge-form")) {
		return "", errors.New("source requires verification")
	}
	match := streamPattern.FindSubmatch(body)
	if len(match) != 2 || !validTarget(string(match[1])) {
		return "", errors.New("stream unavailable")
	}
	address, _ := url.Parse(string(match[1]))
	if !strings.HasSuffix(strings.ToLower(address.Path), ".m3u8") {
		return "", errors.New("stream unavailable")
	}
	return address.String(), nil
}

func (s *Service) Sign(address, referer string, expires time.Time) (string, error) {
	if !validTarget(address) || !validTarget(referer) {
		return "", errors.New("invalid playback target")
	}
	data, err := json.Marshal(ticket{URL: address, Referer: referer, Expires: expires.Unix()})
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.secret[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, data, nil)), nil
}

func (s *Service) Verify(value string) (string, string, error) {
	if len(value) > 8192 {
		return "", "", errors.New("invalid playback token")
	}
	block, err := aes.NewCipher(s.secret[:])
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	encoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(encoded) < gcm.NonceSize() {
		return "", "", errors.New("invalid playback token")
	}
	data, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
	var result ticket
	if err != nil || json.Unmarshal(data, &result) != nil || result.Expires <= time.Now().Unix() || !validTarget(result.URL) || !validTarget(result.Referer) {
		return "", "", errors.New("expired or invalid playback token")
	}
	return result.URL, result.Referer, nil
}

func (s *Service) RewritePlaylist(body []byte, baseURL, referer string, expiry time.Time) ([]byte, error) {
	playlist, _, err := m3u8.DecodeFrom(bytes.NewReader(body), true)
	if err != nil {
		return nil, errors.New("invalid playback playlist")
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, errors.New("invalid playback playlist")
	}
	rewrite := func(value *string) error {
		if *value == "" {
			return nil
		}
		reference, err := url.Parse(*value)
		if err != nil {
			return err
		}
		token, err := s.Sign(base.ResolveReference(reference).String(), referer, expiry)
		if err != nil {
			return err
		}
		*value = "/api/v1/playback/relay/" + token
		return nil
	}
	switch value := playlist.(type) {
	case *m3u8.MasterPlaylist:
		if value.ContentSteering != nil || len(value.Defines) > 0 || len(value.SessionDatas) > 0 {
			return nil, errors.New("unsupported playback playlist")
		}
		for _, key := range value.SessionKeys {
			if err := rewrite(&key.URI); err != nil {
				return nil, err
			}
		}
		for _, variant := range value.Variants {
			if err := rewrite(&variant.URI); err != nil {
				return nil, err
			}
			for _, alternative := range variant.Alternatives {
				if err := rewrite(&alternative.URI); err != nil {
					return nil, err
				}
			}
		}
	case *m3u8.MediaPlaylist:
		if len(value.Defines) > 0 || len(value.PartialSegments) > 0 || value.PreloadHints != nil {
			return nil, errors.New("unsupported playback playlist")
		}
		for i := range value.Keys {
			if err := rewrite(&value.Keys[i].URI); err != nil {
				return nil, err
			}
		}
		if value.Map != nil {
			if err := rewrite(&value.Map.URI); err != nil {
				return nil, err
			}
		}
		for _, segment := range value.Segments {
			if segment == nil {
				continue
			}
			if err := rewrite(&segment.URI); err != nil {
				return nil, err
			}
			for i := range segment.Keys {
				if err := rewrite(&segment.Keys[i].URI); err != nil {
					return nil, err
				}
			}
			if segment.Map != nil {
				if err := rewrite(&segment.Map.URI); err != nil {
					return nil, err
				}
			}
		}
	default:
		return nil, errors.New("unsupported playback playlist")
	}
	return playlist.Encode().Bytes(), nil
}
