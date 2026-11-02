package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (s Service) sendHTTP(ctx context.Context, ch Channel, msg string) error {
	key := "message"
	if ch.BodyShape == BodyShapeText {
		key = "text"
	}
	body, _ := json.Marshal(map[string]string{key: msg})
	return s.post(ctx, ch.Target, body, "")
}

func (s Service) sendSMS(ctx context.Context, ch Channel, to, msg string) error {
	if !httpsOrLoopback(ch.Target) {
		return errors.New("sms-http target must be an https URL; nothing sent")
	}
	auth := ""
	if ch.AuthEnv != "" {
		if auth = s.getenv(ch.AuthEnv); auth == "" {
			return fmt.Errorf("credentials not available in %s; nothing sent", ch.AuthEnv)
		}
	}
	body := renderTemplate(ch.BodyTemplate, to, msg)
	if !json.Valid([]byte(body)) {
		return errors.New("body_template did not render to JSON; nothing sent")
	}
	return s.post(ctx, ch.Target, []byte(body), auth)
}

// renderTemplate substitutes JSON-string-escaped values in one pass, so text
// inside the message is never expanded as a placeholder.
func renderTemplate(tmpl, to, msg string) string {
	return strings.NewReplacer("{to}", jsonEscape(to), "{message}", jsonEscape(msg)).Replace(tmpl)
}

func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// post sends to the configured target only: redirects are not followed, and
// the response body is discarded unread into the outcome.
func (s Service) post(ctx context.Context, target string, body []byte, auth string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	base := s.Client
	if base == nil {
		base = &http.Client{}
	}
	client := *base
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	return nil
}
