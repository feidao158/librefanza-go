package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	gomail "github.com/wneessen/go-mail"
)

type Settings struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	BaseURL  string `json:"base_url"`
	TLSMode  string `json:"tls_mode"`
}

func Validate(cfg Settings) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Host == "" || cfg.Port < 1 || cfg.Port > 65535 || (cfg.TLSMode != "starttls" && cfg.TLSMode != "tls") {
		return errors.New("invalid SMTP settings")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return errors.New("invalid sender")
	}
	_, err := ResetLink(cfg.BaseURL, "test")
	return err
}

func ResetLink(baseURL, token string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return "", errors.New("invalid application URL")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/reset-password"
	values := url.Values{}
	values.Set("token", token)
	base.RawQuery = values.Encode()
	return base.String(), nil
}

func SendReset(ctx context.Context, cfg Settings, recipient, token string) error {
	return sendReset(ctx, cfg, recipient, token, nil)
}

func sendReset(ctx context.Context, cfg Settings, recipient, token string, tlsConfig *tls.Config) error {
	message, err := resetMessage(cfg, recipient, token)
	if err != nil {
		return err
	}
	options := []gomail.Option{gomail.WithPort(cfg.Port), gomail.WithTimeout(10 * time.Second)}
	if tlsConfig != nil {
		options = append(options, gomail.WithTLSConfig(tlsConfig))
	}
	if cfg.TLSMode == "tls" {
		options = append(options, gomail.WithSSL())
	} else {
		options = append(options, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}
	if cfg.Username != "" {
		options = append(options, gomail.WithSMTPAuth(gomail.SMTPAuthPlain), gomail.WithUsername(cfg.Username), gomail.WithPassword(cfg.Password))
	}
	client, err := gomail.NewClient(cfg.Host, options...)
	if err != nil {
		return errors.New("invalid SMTP client")
	}
	if err := client.DialAndSendWithContext(ctx, message); err != nil {
		return errors.New("SMTP delivery failed")
	}
	return nil
}

func resetMessage(cfg Settings, recipient, token string) (*gomail.Msg, error) {
	if !cfg.Enabled {
		return nil, errors.New("SMTP is disabled")
	}
	if err := Validate(cfg); err != nil {
		return nil, err
	}
	link, err := ResetLink(cfg.BaseURL, token)
	if err != nil {
		return nil, err
	}
	message := gomail.NewMsg()
	if err := message.From(cfg.From); err != nil {
		return nil, err
	}
	if err := message.To(recipient); err != nil {
		return nil, err
	}
	message.Subject("LibreFanza password reset")
	message.SetBodyString(gomail.TypeTextPlain, fmt.Sprintf("Use this link to reset your password. It expires in one hour:\n\n%s\n", link))
	return message, nil
}
