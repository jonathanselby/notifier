package notify

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Validate checks trusted deployment configuration. Errors name channels,
// never addresses.
func Validate(c Config) error {
	if len(c.Channels) == 0 {
		return errors.New("at least one channel is required")
	}
	seen := map[string]bool{}
	for _, ch := range c.Channels {
		name := strings.TrimSpace(ch.Name)
		if name == "" || strings.TrimSpace(ch.Target) == "" || seen[name] {
			return errors.New("invalid channel configuration")
		}
		seen[name] = true
		if err := validateChannel(ch); err != nil {
			return fmt.Errorf("channel %q: %w", name, err)
		}
	}
	return nil
}

func validateChannel(ch Channel) error {
	switch ch.Type {
	case TypeFile, TypeHTTP, TypeEmail, TypeSMSHTTP:
	default:
		return errors.New("unsupported channel type")
	}
	if ch.External != nil && *ch.External != IsExternal(ch.Type) {
		if IsExternal(ch.Type) {
			return fmt.Errorf("%s channel is always external", ch.Type)
		}
		return errors.New("file channel cannot be external")
	}
	if ch.Addressing != "" && ch.Addressing != requiredAddressing(ch.Type) {
		return fmt.Errorf("%s channel addressing must be %q", ch.Type, requiredAddressing(ch.Type))
	}
	if addressing(ch) == AddressingPerRecipient {
		if _, err := compilePattern(ch.AddressPattern); err != nil {
			return err
		}
	} else if ch.AddressPattern != "" {
		return errors.New("address_pattern applies only to per_recipient channels")
	}
	if ch.BodyTemplate != "" && ch.Type != TypeSMSHTTP {
		return errors.New("body_template applies only to sms-http channels")
	}
	if ch.BodyShape != "" && ch.Type != TypeHTTP {
		return errors.New("body_shape applies only to http channels")
	}
	if (ch.From != "" || ch.Subject != "") && ch.Type != TypeEmail {
		return errors.New("from and subject apply only to email channels")
	}
	if ch.AuthEnv != "" {
		if ch.Type != TypeEmail && ch.Type != TypeSMSHTTP {
			return errors.New("auth_env applies only to email and sms-http channels")
		}
		if !envName.MatchString(ch.AuthEnv) {
			return errors.New("auth_env must be an environment variable name")
		}
	}
	switch ch.Type {
	case TypeHTTP:
		if ch.BodyShape != "" && ch.BodyShape != BodyShapeMessage && ch.BodyShape != BodyShapeText {
			return fmt.Errorf("body_shape must be %q or %q", BodyShapeMessage, BodyShapeText)
		}
	case TypeSMSHTTP:
		if !httpsOrLoopback(ch.Target) {
			return errors.New("sms-http target must be an https URL")
		}
		if !strings.Contains(ch.BodyTemplate, "{to}") || !strings.Contains(ch.BodyTemplate, "{message}") {
			return errors.New("body_template must contain {to} and {message}")
		}
		if !json.Valid([]byte(renderTemplate(ch.BodyTemplate, "+10000000000", "x \"y\"\n"))) {
			return errors.New("body_template must render to JSON")
		}
	case TypeEmail:
		host, port, err := net.SplitHostPort(ch.Target)
		if err != nil || host == "" || port == "" {
			return errors.New("email target must be host:port of an SMTP submission server")
		}
		if !bareMailbox(ch.From) {
			return errors.New("email from must be a bare address")
		}
		if strings.ContainsAny(ch.Subject, "\r\n") {
			return errors.New("email subject must be one line")
		}
	}
	return nil
}

// compilePattern requires an anchored pattern so a channel cannot become an
// open relay through an unanchored expression.
func compilePattern(p string) (*regexp.Regexp, error) {
	if p == "" {
		return nil, errors.New("per_recipient channel requires address_pattern")
	}
	if !strings.HasPrefix(p, "^") || !strings.HasSuffix(p, "$") {
		return nil, errors.New("address_pattern must be anchored with ^ and $")
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, errors.New("address_pattern is not a valid regular expression")
	}
	return re, nil
}

func bareMailbox(s string) bool {
	if s == "" || hasControlOrSpace(s) {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Name == "" && a.Address == s
}

func httpsOrLoopback(target string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return u.Scheme == "http"
	}
	return false
}

// ValidateRequest checks a request without configuration. Errors name
// channels, never addresses.
func ValidateRequest(r Request) error {
	if strings.TrimSpace(r.Message) == "" {
		return errors.New("message is required")
	}
	if len(r.Channels) == 0 && len(r.Deliveries) == 0 {
		return errors.New("at least one requested channel or delivery is required")
	}
	if r.TimeoutMS < 0 || r.TimeoutMS > MaxTimeoutMS {
		return fmt.Errorf("timeout_ms must be between 1 and %d when supplied", MaxTimeoutMS)
	}
	// One key per destination: a channel name alone for fixed sends, or the
	// channel plus address. A repeated key would send twice.
	targets := map[string]bool{}
	for _, name := range r.Channels {
		if strings.TrimSpace(name) == "" || targets[name+"\x00"] {
			return errors.New("requested channel names must be non-empty and unique")
		}
		targets[name+"\x00"] = true
	}
	refs := map[string]bool{}
	for i, d := range r.Deliveries {
		if strings.TrimSpace(d.Channel) == "" {
			return fmt.Errorf("delivery %d: channel is required", i+1)
		}
		if strings.TrimSpace(d.Ref) == "" || len(d.Ref) > MaxRefLength || hasControlOrSpace(d.Ref) {
			return fmt.Errorf("delivery %d: ref is required (at most %d characters, no spaces)", i+1, MaxRefLength)
		}
		if len(d.To) > MaxAddressLength {
			return fmt.Errorf("delivery %d (ref %s): address too long", i+1, d.Ref)
		}
		if refs[d.Channel+"\x00"+d.Ref] {
			return fmt.Errorf("delivery %d: ref %s repeated on channel %q", i+1, d.Ref, d.Channel)
		}
		refs[d.Channel+"\x00"+d.Ref] = true
		key := d.Channel + "\x00" + strings.ToLower(d.To)
		if targets[key] {
			return fmt.Errorf("delivery %d (ref %s): same destination requested twice on channel %q", i+1, d.Ref, d.Channel)
		}
		targets[key] = true
	}
	return nil
}
