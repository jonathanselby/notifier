package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	DefaultTimeoutMS = 3000
	MaxTimeoutMS     = 60000
)

type Channel struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	External bool   `json:"external,omitempty"`
}

type Config struct {
	Channels []Channel `json:"channels"`
}

type Request struct {
	Message   string   `json:"message"`
	Channels  []string `json:"channels"`
	Exercise  bool     `json:"exercise,omitempty"`
	TimeoutMS int      `json:"timeout_ms,omitempty"`
}

type Outcome struct {
	Channel    string `json:"channel"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

type Result struct {
	Outcomes     []Outcome `json:"outcomes"`
	AllSucceeded bool      `json:"all_succeeded"`
}

type Service struct{ Client *http.Client }

func (s Service) Send(ctx context.Context, c Config, r Request) Result {
	res := Result{AllSucceeded: true}
	if r.TimeoutMS == 0 {
		r.TimeoutMS = DefaultTimeoutMS
	}
	for _, name := range r.Channels {
		ch, ok := find(c.Channels, name)
		if !ok {
			res.Outcomes = append(res.Outcomes, Outcome{Channel: name, Status: "failed", Detail: "channel not configured"})
			res.AllSucceeded = false
			continue
		}
		if r.Exercise && isExternal(ch) {
			res.Outcomes = append(res.Outcomes, Outcome{Channel: name, Status: "suppressed", Detail: "external channel suppressed in exercise mode"})
			continue
		}
		start := time.Now()
		cctx, cancel := context.WithTimeout(ctx, time.Duration(r.TimeoutMS)*time.Millisecond)
		err := s.sendOne(cctx, ch, r.Message)
		cancel()
		o := Outcome{Channel: name, DurationMS: time.Since(start).Milliseconds()}
		if err != nil {
			o.Status = "failed"
			o.Detail = err.Error()
			res.AllSucceeded = false
		} else {
			o.Status = "delivered"
		}
		res.Outcomes = append(res.Outcomes, o)
	}
	return res
}

func (s Service) sendOne(ctx context.Context, ch Channel, msg string) error {
	switch ch.Type {
	case "file":
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		f, err := os.OpenFile(ch.Target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintln(f, msg)
		return err
	case "http":
		body, _ := json.Marshal(map[string]string{"message": msg})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ch.Target, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
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
	default:
		return errors.New("unsupported channel type")
	}
}

func isExternal(ch Channel) bool {
	return ch.Type == "http"
}

func find(xs []Channel, name string) (Channel, bool) {
	for _, x := range xs {
		if x.Name == name {
			return x, true
		}
	}
	return Channel{}, false
}

func Validate(c Config) error {
	if len(c.Channels) == 0 {
		return errors.New("at least one channel is required")
	}
	seen := map[string]bool{}
	for _, ch := range c.Channels {
		name := strings.TrimSpace(ch.Name)
		target := strings.TrimSpace(ch.Target)
		if name == "" || target == "" || seen[name] {
			return errors.New("invalid channel configuration")
		}
		switch ch.Type {
		case "file":
			if ch.External {
				return errors.New("file channel cannot be external")
			}
		case "http":
			if !ch.External {
				return errors.New("http channel must be marked external")
			}
		default:
			return errors.New("invalid channel configuration")
		}
		seen[name] = true
	}
	return nil
}

func ValidateRequest(r Request) error {
	if strings.TrimSpace(r.Message) == "" {
		return errors.New("message is required")
	}
	if len(r.Channels) == 0 {
		return errors.New("at least one requested channel is required")
	}
	if r.TimeoutMS < 0 || r.TimeoutMS > MaxTimeoutMS {
		return fmt.Errorf("timeout_ms must be between 1 and %d when supplied", MaxTimeoutMS)
	}
	seen := map[string]bool{}
	for _, name := range r.Channels {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return errors.New("requested channel names must be non-empty and unique")
		}
		seen[name] = true
	}
	return nil
}
