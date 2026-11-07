package sources

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
)

type RequestError struct {
	Reason  string
	Status  int
	RetryAt time.Time
}

func (e *RequestError) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("source returned HTTP %d", e.Status)
	}
	switch e.Reason {
	case "dns":
		return "source request failed: DNS resolution failed"
	case "timeout":
		return "source request failed: timeout"
	case "connection_refused":
		return "source request failed: connection refused"
	case "connection_reset":
		return "source request failed: connection reset"
	case "tls":
		return "source request failed: TLS error"
	case "cancelled":
		return "source request cancelled"
	default:
		return "source request failed"
	}
}

func requestError(err error) error {
	var dns *net.DNSError
	var network net.Error
	var unknownAuthority x509.UnknownAuthorityError
	var invalidCertificate x509.CertificateInvalidError
	var invalidHostname x509.HostnameError
	var tlsRecord tls.RecordHeaderError
	reason := "network"
	switch {
	case errors.Is(err, context.Canceled):
		reason = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		reason = "timeout"
	case errors.As(err, &dns):
		reason = "dns"
	case errors.Is(err, syscall.ECONNREFUSED):
		reason = "connection_refused"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE):
		reason = "connection_reset"
	case errors.As(err, &unknownAuthority), errors.As(err, &invalidCertificate), errors.As(err, &invalidHostname), errors.As(err, &tlsRecord):
		reason = "tls"
	case errors.As(err, &network) && network.Timeout():
		reason = "timeout"
	}
	return &RequestError{Reason: reason}
}

type CrawlError struct {
	PositionType string
	Position     int
	Stage        string
	Err          error
}

func (e *CrawlError) Error() string {
	return fmt.Sprintf("crawl %s %d %s: %v", e.PositionType, e.Position, e.Stage, e.Err)
}

func (e *CrawlError) Unwrap() error { return e.Err }
