package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"
)

// sendEmail submits one message over SMTP. STARTTLS is required: nothing is
// sent, and no credentials are offered, on a connection that is not encrypted.
func (s Service) sendEmail(ctx context.Context, ch Channel, to, msg string) error {
	host, _, err := net.SplitHostPort(ch.Target)
	if err != nil {
		return errors.New("email target must be host:port")
	}
	var user, pass string
	if ch.AuthEnv != "" {
		creds := s.getenv(ch.AuthEnv)
		var ok bool
		if user, pass, ok = strings.Cut(creds, ":"); !ok || user == "" {
			return fmt.Errorf("credentials not available in %s (want user:password); nothing sent", ch.AuthEnv)
		}
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", ch.Target)
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return smtpError(ctx, "greeting", err)
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); !ok {
		if ctx.Err() != nil {
			return smtpError(ctx, "hello", ctx.Err())
		}
		return errors.New("smtp server did not offer STARTTLS; nothing sent")
	}
	cfg := &tls.Config{}
	if s.TLSConfig != nil {
		cfg = s.TLSConfig.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}
	if cfg.MinVersion < tls.VersionTLS12 {
		cfg.MinVersion = tls.VersionTLS12
	}
	if err := c.StartTLS(cfg); err != nil {
		return smtpError(ctx, "starttls", err)
	}
	if ch.AuthEnv != "" {
		if ok, _ := c.Extension("AUTH"); !ok {
			return errors.New("smtp server offered no authentication; nothing sent")
		}
		if err := c.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
			return smtpError(ctx, "authentication", err)
		}
	}
	if err := c.Mail(ch.From); err != nil {
		return smtpError(ctx, "sender", err)
	}
	if err := c.Rcpt(to); err != nil {
		return smtpError(ctx, "recipient", err)
	}
	w, err := c.Data()
	if err != nil {
		return smtpError(ctx, "data", err)
	}
	if _, err := w.Write(composeEmail(ch, to, msg)); err != nil {
		return smtpError(ctx, "data", err)
	}
	if err := w.Close(); err != nil {
		return smtpError(ctx, "data", err)
	}
	_ = c.Quit()
	return nil
}

// smtpError reports a server refusal by stage and reply code only. Server
// reply text is never copied, because servers echo the recipient address.
func smtpError(ctx context.Context, stage string, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("smtp %s: %w", stage, ctx.Err())
	}
	// The socket deadline equals the context deadline and may fire first.
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return fmt.Errorf("smtp %s: %w", stage, context.DeadlineExceeded)
	}
	var te *textproto.Error
	if errors.As(err, &te) {
		return fmt.Errorf("smtp %s refused (%d)", stage, te.Code)
	}
	if stage == "recipient" || stage == "data" {
		return fmt.Errorf("smtp %s failed", stage)
	}
	return fmt.Errorf("smtp %s: %w", stage, err)
}

func composeEmail(ch Channel, to, msg string) []byte {
	subject := ch.Subject
	if subject == "" {
		subject = "Notification"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", ch.From)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&b)
	body := strings.ReplaceAll(strings.ReplaceAll(msg, "\r\n", "\n"), "\n", "\r\n")
	_, _ = qp.Write([]byte(body))
	_ = qp.Close()
	b.WriteString("\r\n")
	return b.Bytes()
}
