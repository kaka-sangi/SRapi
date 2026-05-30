package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
)

const (
	smtpDialTimeout = 10 * time.Second
	smtpIOTimeout   = 20 * time.Second
)

type SMTPSender struct {
	config notificationscontract.EmailConfig
}

func NewSMTPSender(cfg notificationscontract.EmailConfig) *SMTPSender {
	if cfg.SMTPPort <= 0 {
		cfg.SMTPPort = 587
	}
	return &SMTPSender{config: cfg}
}

func (s *SMTPSender) Send(ctx context.Context, message notificationscontract.EmailMessage) error {
	if s == nil {
		return ErrNotConfigured
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg := s.config
	if strings.TrimSpace(cfg.SMTPHost) == "" || strings.TrimSpace(cfg.SMTPFrom) == "" {
		return ErrNotConfigured
	}
	to := sanitizeHeader(message.To)
	subject := sanitizeHeader(message.Subject)
	from := sanitizeHeader(cfg.SMTPFrom)
	if cfg.SMTPFromName != "" {
		from = fmt.Sprintf("%s <%s>", sanitizeHeader(cfg.SMTPFromName), sanitizeHeader(cfg.SMTPFrom))
	}
	headers := []string{
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
	}
	for _, header := range safeSMTPHeaders(message.Headers) {
		headers = append(headers, header)
	}
	headers = append(headers, "MIME-Version: 1.0", "Content-Type: text/html; charset=UTF-8")
	raw := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + message.HTML)
	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	var auth smtp.Auth
	if strings.TrimSpace(cfg.SMTPUsername) != "" || strings.TrimSpace(cfg.SMTPPassword) != "" {
		auth = smtp.PlainAuth("", cfg.SMTPUsername, cfg.SMTPPassword, cfg.SMTPHost)
	}
	if cfg.SMTPUseTLS {
		return sendMailTLS(addr, auth, cfg.SMTPFrom, to, raw, cfg.SMTPHost)
	}
	return sendMailPlain(addr, auth, cfg.SMTPFrom, to, raw, cfg.SMTPHost)
}

func safeSMTPHeaders(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, 2)
	for _, name := range []string{"List-Unsubscribe", "List-Unsubscribe-Post"} {
		value := sanitizeHeader(values[name])
		if value == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s", name, value))
	}
	return out
}

func sendMailPlain(addr string, auth smtp.Auth, from, to string, msg []byte, host string) error {
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpIOTimeout))
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	return sendMailWithClient(client, auth, from, to, msg)
}

func sendMailTLS(addr string, auth smtp.Auth, from, to string, msg []byte, host string) error {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: smtpDialTimeout}, "tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("smtp tls dial: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(smtpIOTimeout))
	defer func() { _ = conn.Close() }()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()
	return sendMailWithClient(client, auth, from, to, msg)
}

func sendMailWithClient(client *smtp.Client, auth smtp.Auth, from, to string, msg []byte) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp mail: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp close: %w", err)
	}
	_ = client.Quit()
	return nil
}
