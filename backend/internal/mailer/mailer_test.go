package mailer

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResetLinkUsesConfiguredBaseURL(t *testing.T) {
	link, err := ResetLink("https://example.com/app", "abc+123")
	if err != nil || !strings.HasPrefix(link, "https://example.com/app/reset-password?") || !strings.Contains(link, "token=abc%2B123") {
		t.Fatalf("link=%q err=%v", link, err)
	}
	if _, err := ResetLink("javascript:alert(1)", "abc"); err == nil {
		t.Fatal("invalid base URL accepted")
	}
}

func TestSendResetOverTLS(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(certificate)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			received <- ""
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		reader := bufio.NewReader(conn)
		_, _ = io.WriteString(conn, "220 localhost ESMTP\r\n")
		var body strings.Builder
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				received <- body.String()
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					_, _ = io.WriteString(conn, "250 queued\r\n")
					continue
				}
				body.WriteString(line)
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				_, _ = io.WriteString(conn, "250-localhost\r\n250 SIZE 1048576\r\n")
			case strings.HasPrefix(line, "MAIL FROM"), strings.HasPrefix(line, "RCPT TO"):
				_, _ = io.WriteString(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "DATA"):
				inData = true
				_, _ = io.WriteString(conn, "354 continue\r\n")
			case strings.HasPrefix(line, "QUIT"):
				_, _ = io.WriteString(conn, "221 bye\r\n")
				received <- body.String()
				return
			default:
				_, _ = io.WriteString(conn, "250 OK\r\n")
			}
		}
	}()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	cfg := Settings{Enabled: true, Host: "127.0.0.1", Port: number, From: "sender@example.com", BaseURL: "https://example.com", TLSMode: "tls"}
	if err := sendReset(context.Background(), cfg, "user@example.com", "token", &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}); err != nil {
		t.Fatal(err)
	}
	select {
	case body := <-received:
		if !strings.Contains(body, "user@example.com") {
			t.Fatalf("SMTP message missing recipient: %s", body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("SMTP server did not receive message")
	}
}

func TestResetMessageUsesRecipientAndConfiguredURL(t *testing.T) {
	cfg := Settings{Enabled: true, Host: "smtp.example.com", Port: 587, From: "sender@example.com", BaseURL: "https://example.com", TLSMode: "starttls"}
	message, err := resetMessage(cfg, "user@example.com", "secret-token")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if _, err := message.WriteTo(&body); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"user@example.com", "sender@example.com"} {
		if !strings.Contains(body.String(), value) {
			t.Fatalf("message does not contain %q", value)
		}
	}
	parsed, err := mail.ReadMessage(&body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	if err != nil || !strings.Contains(string(decoded), "https://example.com/reset-password?token=secret-token") {
		t.Fatalf("invalid message body: %q, %v", decoded, err)
	}
}
