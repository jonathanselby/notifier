// Package notify attempts an already-composed notification through configured
// channels and records one outcome per requested delivery.
//
// Delivery is an outcome to record, not an assumption to hide.
package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultTimeoutMS = 3000
	MaxTimeoutMS     = 60000

	// MaxAddressLength bounds a per-recipient address (RFC 5321 path limit).
	MaxAddressLength = 320
	// MaxRefLength bounds the caller's opaque delivery reference.
	MaxRefLength = 256
)

// Channel types. External is derived from the type: file is never external;
// every other type always is.
const (
	TypeFile    = "file"
	TypeHTTP    = "http"
	TypeEmail   = "email"
	TypeSMSHTTP = "sms-http"
)

// Addressing modes. file and http channels deliver to their configured target;
// email and sms-http channels deliver to an address supplied per delivery.
const (
	AddressingFixed        = "fixed"
	AddressingPerRecipient = "per_recipient"
)

// HTTP body shapes for the http channel type.
const (
	BodyShapeMessage = "message" // {"message": "..."} (v0.1 default)
	BodyShapeText    = "text"    // {"text": "..."} (Slack and Teams incoming webhooks)
)

// Outcome statuses.
const (
	StatusDelivered  = "delivered"
	StatusFailed     = "failed"
	StatusSuppressed = "suppressed"
)

// Channel is trusted deployment configuration.
type Channel struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Target string `json:"target"`
	// External is optional metadata kept for v0.1 configuration. When present
	// it must agree with the type; the runtime never relies on it.
	External       *bool  `json:"external,omitempty"`
	Addressing     string `json:"addressing,omitempty"`
	AddressPattern string `json:"address_pattern,omitempty"`
	// BodyTemplate is the JSON body for sms-http, with {to} and {message}.
	BodyTemplate string `json:"body_template,omitempty"`
	// BodyShape selects the JSON body for http: "message" (default) or "text".
	BodyShape string `json:"body_shape,omitempty"`
	// From is the fixed sender for email.
	From string `json:"from,omitempty"`
	// Subject is the fixed subject for email; "Notification" when empty.
	Subject string `json:"subject,omitempty"`
	// AuthEnv names the environment variable holding credentials: for email
	// "user:password" (SMTP AUTH PLAIN after STARTTLS); for sms-http the value
	// of the Authorization header.
	AuthEnv string `json:"auth_env,omitempty"`
}

type Config struct {
	Channels []Channel `json:"channels"`
}

// Delivery addresses one notification to one channel. To is required for
// per-recipient channels and must be empty for fixed channels. Ref is the
// caller's opaque reference, echoed in the outcome in place of the address.
type Delivery struct {
	Channel string `json:"channel"`
	To      string `json:"to,omitempty"`
	Ref     string `json:"ref"`
}

// Request carries an already-composed message. Channels (v0.1 form) names
// fixed channels; Deliveries addresses per-recipient or fixed channels.
// Outcomes follow Channels first, then Deliveries, each in the given order.
type Request struct {
	Message    string     `json:"message"`
	Channels   []string   `json:"channels,omitempty"`
	Deliveries []Delivery `json:"deliveries,omitempty"`
	Exercise   bool       `json:"exercise,omitempty"`
	TimeoutMS  int        `json:"timeout_ms,omitempty"`
}

// Outcome never carries the delivery address: Ref identifies the delivery.
type Outcome struct {
	Channel    string `json:"channel"`
	Ref        string `json:"ref,omitempty"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

type Result struct {
	Outcomes     []Outcome `json:"outcomes"`
	AllSucceeded bool      `json:"all_succeeded"`
}

// Service performs deliveries. The zero value is ready to use.
type Service struct {
	// Client is the base HTTP client for http and sms-http channels.
	Client *http.Client
	// TLSConfig is the base TLS configuration for SMTP STARTTLS. ServerName
	// defaults to the SMTP target host.
	TLSConfig *tls.Config
	// Getenv reads credentials named by AuthEnv; os.Getenv when nil.
	Getenv func(string) string
}

// IsExternal reports whether a channel type reaches outside the host.
func IsExternal(channelType string) bool {
	return channelType != TypeFile
}

func addressing(ch Channel) string {
	if ch.Addressing != "" {
		return ch.Addressing
	}
	return requiredAddressing(ch.Type)
}

func requiredAddressing(channelType string) string {
	switch channelType {
	case TypeEmail, TypeSMSHTTP:
		return AddressingPerRecipient
	default:
		return AddressingFixed
	}
}

type item struct {
	channel string
	to      string
	ref     string
}

// Send attempts every requested delivery and returns exactly one outcome for
// each. It never retries. Address checks and exercise suppression happen
// before any I/O.
func (s Service) Send(ctx context.Context, c Config, r Request) Result {
	res := Result{AllSucceeded: true, Outcomes: []Outcome{}}
	if r.TimeoutMS == 0 {
		r.TimeoutMS = DefaultTimeoutMS
	}
	items := make([]item, 0, len(r.Channels)+len(r.Deliveries))
	for _, name := range r.Channels {
		items = append(items, item{channel: name})
	}
	for _, d := range r.Deliveries {
		items = append(items, item{channel: d.Channel, to: d.To, ref: d.Ref})
	}
	patterns := map[string]*regexp.Regexp{}
	for _, it := range items {
		o := s.deliver(ctx, c, r, it, patterns)
		o.Channel = it.channel
		o.Ref = it.ref
		o.Detail = scrub(o.Detail, it.to)
		if o.Status == StatusFailed {
			res.AllSucceeded = false
		}
		res.Outcomes = append(res.Outcomes, o)
	}
	return res
}

func (s Service) deliver(ctx context.Context, c Config, r Request, it item, patterns map[string]*regexp.Regexp) Outcome {
	ch, ok := find(c.Channels, it.channel)
	if !ok {
		return Outcome{Status: StatusFailed, Detail: "channel not configured"}
	}
	if detail := checkAddress(ch, it.to, patterns); detail != "" {
		return Outcome{Status: StatusFailed, Detail: detail}
	}
	if r.Exercise && IsExternal(ch.Type) {
		return Outcome{Status: StatusSuppressed, Detail: "external channel suppressed in exercise mode"}
	}
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, time.Duration(r.TimeoutMS)*time.Millisecond)
	err := s.sendOne(cctx, ch, it.to, r.Message)
	cancel()
	o := Outcome{DurationMS: time.Since(start).Milliseconds()}
	if err != nil {
		o.Status = StatusFailed
		o.Detail = err.Error()
	} else {
		o.Status = StatusDelivered
	}
	return o
}

// checkAddress applies the addressing rule and the channel's allow-pattern.
// It returns a refusal detail, or "" when the delivery may proceed.
func checkAddress(ch Channel, to string, patterns map[string]*regexp.Regexp) string {
	if addressing(ch) != AddressingPerRecipient {
		if to != "" {
			return "fixed channel takes no address"
		}
		return ""
	}
	if to == "" {
		return "address required"
	}
	if len(to) > MaxAddressLength || hasControlOrSpace(to) {
		return "address refused"
	}
	re, ok := patterns[ch.Name]
	if !ok {
		re, _ = compilePattern(ch.AddressPattern)
		patterns[ch.Name] = re
	}
	if re == nil || !re.MatchString(to) {
		return "address refused"
	}
	if ch.Type == TypeEmail && !bareMailbox(to) {
		return "address refused"
	}
	return ""
}

func (s Service) sendOne(ctx context.Context, ch Channel, to, msg string) error {
	switch ch.Type {
	case TypeFile:
		return sendFile(ctx, ch.Target, msg)
	case TypeHTTP:
		return s.sendHTTP(ctx, ch, msg)
	case TypeSMSHTTP:
		return s.sendSMS(ctx, ch, to, msg)
	case TypeEmail:
		return s.sendEmail(ctx, ch, to, msg)
	default:
		return errors.New("unsupported channel type")
	}
}

func sendFile(ctx context.Context, target, msg string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, msg)
	return err
}

func (s Service) getenv(name string) string {
	if s.Getenv != nil {
		return s.Getenv(name)
	}
	return os.Getenv(name)
}

// scrub removes any occurrence of the address from a detail, as a final guard:
// adapters already build details without it.
func scrub(detail, to string) string {
	if to == "" || detail == "" {
		return detail
	}
	lower, needle := strings.ToLower(detail), strings.ToLower(to)
	var b strings.Builder
	for {
		i := strings.Index(lower, needle)
		if i < 0 {
			b.WriteString(detail)
			return b.String()
		}
		b.WriteString(detail[:i])
		b.WriteString("[recipient]")
		detail, lower = detail[i+len(needle):], lower[i+len(needle):]
	}
}

func hasControlOrSpace(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func find(xs []Channel, name string) (Channel, bool) {
	for _, x := range xs {
		if x.Name == name {
			return x, true
		}
	}
	return Channel{}, false
}
