package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	emailPattern = `^[^@\s]+@forensic\.example$`
	phonePattern = `^\+[1-9][0-9]{7,14}$`
	priya        = "priya@forensic.example"
	tom          = "+61400000789"
)

func emailChannel(addr string) Channel {
	return Channel{Name: "mail", Type: TypeEmail, Target: addr, Addressing: AddressingPerRecipient,
		AddressPattern: emailPattern, From: "crab@forensic.example", Subject: "CRAB needs you"}
}

func smsChannel(target string) Channel {
	return Channel{Name: "sms", Type: TypeSMSHTTP, Target: target, Addressing: AddressingPerRecipient,
		AddressPattern: phonePattern, BodyTemplate: `{"to":"{to}","text":"{message}"}`, AuthEnv: "TEST_SMS_AUTH"}
}

type gateway struct {
	srv    *httptest.Server
	hits   atomic.Int32
	mu     sync.Mutex
	bodies []string
	auth   []string
}

func newGateway(t *testing.T, status int, delay time.Duration) *gateway {
	g := &gateway{}
	g.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.hits.Add(1)
		b, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.bodies = append(g.bodies, string(b))
		g.auth = append(g.auth, r.Header.Get("Authorization"))
		g.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
			}
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func assertNoAddress(t *testing.T, r Result, addrs ...string) {
	t.Helper()
	b, _ := json.Marshal(r)
	for _, a := range addrs {
		if strings.Contains(strings.ToLower(string(b)), strings.ToLower(a)) {
			t.Fatalf("address %q leaked into result: %s", a, b)
		}
	}
}

func TestPerRecipientEmailOverSTARTTLS(t *testing.T) {
	smtpd := newTestSMTP(t, func(s *testSMTP) { s.user, s.pass = "crab", "s3cret" })
	ch := emailChannel(smtpd.addr())
	ch.AuthEnv = "TEST_SMTP_AUTH"
	c := Config{Channels: []Channel{ch}}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	req := Request{Message: "CRAB: 6 items on INC-007 are waiting for your decision.\n.\nSign in to CRAB.", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "priya.n"}}, TimeoutMS: 5000}
	if err := ValidateRequest(req); err != nil {
		t.Fatal(err)
	}
	r := smtpd.service().Send(context.Background(), c, req)
	if !r.AllSucceeded || len(r.Outcomes) != 1 || r.Outcomes[0].Status != StatusDelivered || r.Outcomes[0].Ref != "priya.n" || r.Outcomes[0].Channel != "mail" {
		t.Fatalf("%+v", r)
	}
	assertNoAddress(t, r, priya)
	cmds := smtpd.commands()
	sawTLS := false
	for i, cmd := range cmds {
		if strings.HasPrefix(cmd, "STARTTLS") {
			sawTLS = true
		}
		if v := strings.ToUpper(strings.SplitN(cmd, " ", 2)[0]); (v == "AUTH" || v == "MAIL" || v == "RCPT" || v == "DATA") && !smtpd.tls[i] {
			t.Fatalf("%s sent before TLS: %v", v, cmds)
		}
	}
	if !sawTLS || len(smtpd.rcpts) != 1 || smtpd.rcpts[0] != priya || len(smtpd.data) != 1 {
		t.Fatalf("commands %v rcpts %v", cmds, smtpd.rcpts)
	}
	body := smtpd.data[0]
	for _, want := range []string{"From: crab@forensic.example", "To: " + priya, "Subject: CRAB needs you", "Content-Type: text/plain; charset=utf-8", "waiting for your decision"} {
		if !strings.Contains(body, want) {
			t.Fatalf("message missing %q:\n%s", want, body)
		}
	}
}

func TestEmailRequiresSTARTTLS(t *testing.T) {
	smtpd := newTestSMTP(t, func(s *testSMTP) { s.offerTLS = false; s.user, s.pass = "crab", "s3cret" })
	ch := emailChannel(smtpd.addr())
	ch.AuthEnv = "TEST_SMTP_AUTH"
	r := smtpd.service().Send(context.Background(), Config{Channels: []Channel{ch}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "p"}}, TimeoutMS: 5000})
	if r.AllSucceeded || r.Outcomes[0].Status != StatusFailed || !strings.Contains(r.Outcomes[0].Detail, "STARTTLS") {
		t.Fatalf("%+v", r)
	}
	for _, cmd := range smtpd.commands() {
		if v := strings.ToUpper(strings.SplitN(cmd, " ", 2)[0]); v == "AUTH" || v == "MAIL" || v == "RCPT" || v == "DATA" {
			t.Fatalf("%s sent on a plaintext connection", v)
		}
	}
}

func TestEmailUntrustedCertificateFails(t *testing.T) {
	smtpd := newTestSMTP(t, nil)
	svc := Service{} // system roots: the test CA is not trusted
	r := svc.Send(context.Background(), Config{Channels: []Channel{emailChannel(smtpd.addr())}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "p"}}, TimeoutMS: 5000})
	if r.Outcomes[0].Status != StatusFailed || !strings.Contains(r.Outcomes[0].Detail, "starttls") {
		t.Fatalf("%+v", r)
	}
	if len(smtpd.rcpts) != 0 {
		t.Fatal("recipient sent over untrusted TLS")
	}
}

func TestEmailRejectionNeverEchoesAddress(t *testing.T) {
	smtpd := newTestSMTP(t, func(s *testSMTP) { s.rejectRcpt = true })
	r := smtpd.service().Send(context.Background(), Config{Channels: []Channel{emailChannel(smtpd.addr())}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "priya.n"}}, TimeoutMS: 5000})
	if r.Outcomes[0].Status != StatusFailed || r.Outcomes[0].Detail != "smtp recipient refused (550)" {
		t.Fatalf("%+v", r)
	}
	assertNoAddress(t, r, priya, "forensic.example")
}

func TestEmailBadCredentialsAndMissingCredentials(t *testing.T) {
	smtpd := newTestSMTP(t, func(s *testSMTP) { s.user, s.pass = "crab", "right" })
	ch := emailChannel(smtpd.addr())
	ch.AuthEnv = "TEST_SMTP_AUTH"
	svc := smtpd.service()
	svc.Getenv = func(string) string { return "crab:wrong" }
	r := svc.Send(context.Background(), Config{Channels: []Channel{ch}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "p"}}, TimeoutMS: 5000})
	if r.Outcomes[0].Status != StatusFailed || r.Outcomes[0].Detail != "smtp authentication refused (535)" {
		t.Fatalf("%+v", r)
	}
	before := smtpd.conns.Load()
	svc.Getenv = func(string) string { return "" }
	r = svc.Send(context.Background(), Config{Channels: []Channel{ch}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "p"}}, TimeoutMS: 5000})
	if r.Outcomes[0].Status != StatusFailed || !strings.Contains(r.Outcomes[0].Detail, "TEST_SMTP_AUTH") || smtpd.conns.Load() != before {
		t.Fatalf("missing credentials must fail before I/O: %+v", r)
	}
}

func TestPerRecipientSMSHTTP(t *testing.T) {
	g := newGateway(t, http.StatusAccepted, 0)
	c := Config{Channels: []Channel{smsChannel(g.srv.URL)}}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	svc := Service{Client: g.srv.Client(), Getenv: func(string) string { return "Bearer gw-token" }}
	msg := `INC-007: "decision" needed {to} {message}`
	r := svc.Send(context.Background(), c, Request{Message: msg, Deliveries: []Delivery{{Channel: "sms", To: tom, Ref: "tom.b"}}, TimeoutMS: 5000})
	if !r.AllSucceeded || r.Outcomes[0].Status != StatusDelivered || r.Outcomes[0].Ref != "tom.b" {
		t.Fatalf("%+v", r)
	}
	assertNoAddress(t, r, tom)
	var got map[string]string
	if err := json.Unmarshal([]byte(g.bodies[0]), &got); err != nil {
		t.Fatalf("gateway body is not JSON: %q", g.bodies[0])
	}
	if got["to"] != tom || got["text"] != msg || g.auth[0] != "Bearer gw-token" {
		t.Fatalf("body %v auth %q", got, g.auth[0])
	}
	// Gateway refusal is reported by status only.
	g2 := newGateway(t, http.StatusBadGateway, 0)
	r = Service{Client: g2.srv.Client(), Getenv: func(string) string { return "t" }}.Send(context.Background(), Config{Channels: []Channel{smsChannel(g2.srv.URL)}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "sms", To: tom, Ref: "tom.b"}}, TimeoutMS: 5000})
	if r.Outcomes[0].Status != StatusFailed || r.Outcomes[0].Detail != "HTTP status 502" {
		t.Fatalf("%+v", r)
	}
}

func TestHTTPTextBodyShape(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}))
	defer srv.Close()
	c := Config{Channels: []Channel{{Name: "ir-leadership", Type: TypeHTTP, Target: srv.URL, BodyShape: BodyShapeText}}}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	r := Service{}.Send(context.Background(), c, Request{Message: "INC-007 severity raised", Channels: []string{"ir-leadership"}, TimeoutMS: 1000})
	if !r.AllSucceeded || body != `{"text":"INC-007 severity raised"}` {
		t.Fatalf("%+v %s", r, body)
	}
}

func TestPatternRefusalBeforeIO(t *testing.T) {
	smtpd := newTestSMTP(t, nil)
	g := newGateway(t, http.StatusOK, 0)
	c := Config{Channels: []Channel{emailChannel(smtpd.addr()), smsChannel(g.srv.URL)}}
	svc := smtpd.service()
	svc.Client = g.srv.Client()
	svc.Getenv = func(string) string { return "t" }
	bad := []Delivery{
		{Channel: "mail", To: "attacker@elsewhere.example", Ref: "a"},
		{Channel: "mail", To: "x@forensic.example\r\nBcc: victim@elsewhere.example", Ref: "b"},
		{Channel: "mail", To: "Priya <priya@forensic.example>", Ref: "c"},
		{Channel: "mail", To: "", Ref: "d"},
		{Channel: "sms", To: "0400000789", Ref: "e"},
		{Channel: "sms", To: "+61400000789 ", Ref: "f"},
	}
	for _, exercise := range []bool{false, true} {
		r := svc.Send(context.Background(), c, Request{Message: "x", Deliveries: bad, Exercise: exercise, TimeoutMS: 1000})
		if r.AllSucceeded || len(r.Outcomes) != len(bad) {
			t.Fatalf("%+v", r)
		}
		for i, o := range r.Outcomes {
			if o.Status != StatusFailed || (o.Detail != "address refused" && o.Detail != "address required") || o.Ref != bad[i].Ref {
				t.Fatalf("delivery %d: %+v", i, o)
			}
		}
		assertNoAddress(t, r, "attacker@elsewhere.example", "victim", "0400000789")
	}
	if smtpd.conns.Load() != 0 || g.hits.Load() != 0 {
		t.Fatalf("I/O happened for refused addresses: smtp %d gateway %d", smtpd.conns.Load(), g.hits.Load())
	}
}

func TestUnvalidatedPerRecipientChannelIsNeverOpen(t *testing.T) {
	g := newGateway(t, http.StatusOK, 0)
	ch := smsChannel(g.srv.URL)
	for _, p := range []string{"", ".*", "^(unclosed$"} {
		ch.AddressPattern = p
		if Validate(Config{Channels: []Channel{ch}}) == nil {
			t.Fatalf("pattern %q accepted", p)
		}
		r := Service{Client: g.srv.Client()}.Send(context.Background(), Config{Channels: []Channel{ch}}, Request{Message: "x", Deliveries: []Delivery{{Channel: "sms", To: tom, Ref: "t"}}, TimeoutMS: 1000})
		if r.Outcomes[0].Detail != "address refused" {
			t.Fatalf("pattern %q: %+v", p, r)
		}
	}
	if g.hits.Load() != 0 {
		t.Fatal("unvalidated channel contacted")
	}
}

func TestExerciseSuppressesEveryExternalTypeBeforeIO(t *testing.T) {
	smtpd := newTestSMTP(t, nil)
	g := newGateway(t, http.StatusOK, 0)
	hook := newGateway(t, http.StatusOK, 0)
	file := t.TempDir() + "/alerts"
	c := Config{Channels: []Channel{
		{Name: "local", Type: TypeFile, Target: file},
		{Name: "hook", Type: TypeHTTP, Target: hook.srv.URL, BodyShape: BodyShapeText},
		emailChannel(smtpd.addr()),
		smsChannel(g.srv.URL),
	}}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	svc := smtpd.service()
	svc.Client = g.srv.Client()
	r := svc.Send(context.Background(), c, Request{Message: "exercise", Exercise: true, TimeoutMS: 1000, Channels: []string{"local", "hook"},
		Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "priya.n"}, {Channel: "sms", To: tom, Ref: "tom.b"}}})
	want := []string{StatusDelivered, StatusSuppressed, StatusSuppressed, StatusSuppressed}
	for i, o := range r.Outcomes {
		if o.Status != want[i] {
			t.Fatalf("outcome %d: %+v", i, r)
		}
	}
	if !r.AllSucceeded || smtpd.conns.Load() != 0 || g.hits.Load() != 0 || hook.hits.Load() != 0 {
		t.Fatalf("external I/O during exercise: %+v", r)
	}
}

func TestMixedFixedAndPerRecipient(t *testing.T) {
	smtpd := newTestSMTP(t, nil)
	file := t.TempDir() + "/alerts"
	c := Config{Channels: []Channel{{Name: "local", Type: TypeFile, Target: file}, emailChannel(smtpd.addr())}}
	r := smtpd.service().Send(context.Background(), c, Request{Message: "mixed", TimeoutMS: 5000, Channels: []string{"local"},
		Deliveries: []Delivery{
			{Channel: "mail", To: priya, Ref: "priya.n"},
			{Channel: "local", Ref: "ops-log"},
			{Channel: "local", To: priya, Ref: "wrong"},
			{Channel: "mail", Ref: "missing"},
			{Channel: "nowhere", To: priya, Ref: "unknown"},
		}})
	type want struct{ channel, ref, status, detail string }
	expect := []want{
		{"local", "", StatusDelivered, ""},
		{"mail", "priya.n", StatusDelivered, ""},
		{"local", "ops-log", StatusDelivered, ""},
		{"local", "wrong", StatusFailed, "fixed channel takes no address"},
		{"mail", "missing", StatusFailed, "address required"},
		{"nowhere", "unknown", StatusFailed, "channel not configured"},
	}
	if len(r.Outcomes) != len(expect) || r.AllSucceeded {
		t.Fatalf("%+v", r)
	}
	for i, e := range expect {
		o := r.Outcomes[i]
		if o.Channel != e.channel || o.Ref != e.ref || o.Status != e.status || o.Detail != e.detail {
			t.Fatalf("outcome %d = %+v, want %+v", i, o, e)
		}
	}
	assertNoAddress(t, r, priya)
	// A per-recipient channel named in the v0.1 channels list has no address.
	r = smtpd.service().Send(context.Background(), c, Request{Message: "x", Channels: []string{"mail"}, TimeoutMS: 1000})
	if r.Outcomes[0].Detail != "address required" {
		t.Fatalf("%+v", r)
	}
}

func TestV01RequestAndConfigCompatibility(t *testing.T) {
	file := t.TempDir() + "/alerts"
	hook := newGateway(t, http.StatusOK, 0)
	cfgJSON := `{"channels":[{"name":"local-major-users","type":"file","target":"` + file + `","external":false},{"name":"sms-gateway","type":"http","target":"` + hook.srv.URL + `","external":true}]}`
	reqJSON := `{"message":"Bafflewick break-glass activated","channels":["local-major-users","sms-gateway"],"timeout_ms":1000}`
	var c Config
	var req Request
	if err := json.Unmarshal([]byte(cfgJSON), &c); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		t.Fatal(err)
	}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRequest(req); err != nil {
		t.Fatal(err)
	}
	r := Service{Client: hook.srv.Client()}.Send(context.Background(), c, req)
	if !r.AllSucceeded || len(r.Outcomes) != 2 {
		t.Fatalf("%+v", r)
	}
	out, _ := json.Marshal(r)
	if strings.Contains(string(out), `"ref"`) {
		t.Fatalf("v0.1 outcome shape changed: %s", out)
	}
	if hook.bodies[0] != `{"message":"Bafflewick break-glass activated"}` {
		t.Fatalf("v0.1 webhook body changed: %s", hook.bodies[0])
	}
}

func TestTimeoutsBoundedPerDelivery(t *testing.T) {
	slowSMTP := newTestSMTP(t, func(s *testSMTP) { s.hang = true })
	slowGW := newGateway(t, http.StatusOK, 5*time.Second)
	file := t.TempDir() + "/alerts"
	c := Config{Channels: []Channel{emailChannel(slowSMTP.addr()), smsChannel(slowGW.srv.URL), {Name: "local", Type: TypeFile, Target: file}}}
	svc := slowSMTP.service()
	svc.Client = slowGW.srv.Client()
	svc.Getenv = func(string) string { return "t" }
	start := time.Now()
	r := svc.Send(context.Background(), c, Request{Message: "x", TimeoutMS: 100,
		Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "p"}, {Channel: "sms", To: tom, Ref: "t"}, {Channel: "local", Ref: "l"}}})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("deliveries not bounded: %v", elapsed)
	}
	for i := 0; i < 2; i++ {
		if o := r.Outcomes[i]; o.Status != StatusFailed || !strings.Contains(o.Detail, "deadline exceeded") || o.DurationMS > 1000 {
			t.Fatalf("outcome %d: %+v", i, o)
		}
	}
	if r.Outcomes[2].Status != StatusDelivered {
		t.Fatalf("later delivery hidden by earlier timeouts: %+v", r)
	}
	assertNoAddress(t, r, priya, tom)
}

func TestValidatePerRecipientConfig(t *testing.T) {
	good := []Channel{
		emailChannel("smtp.example:587"),
		smsChannel("https://gw.example/send"),
		{Name: "h", Type: TypeHTTP, Target: "https://teams.example/hook", BodyShape: BodyShapeText},
	}
	if err := Validate(Config{Channels: good}); err != nil {
		t.Fatal(err)
	}
	mutate := func(base Channel, f func(*Channel)) Channel { f(&base); return base }
	e, s := emailChannel("smtp.example:587"), smsChannel("https://gw.example/send")
	bad := []Channel{
		mutate(e, func(c *Channel) { c.Target = "smtp.example" }),
		mutate(e, func(c *Channel) { c.From = "CRAB <crab@x.example>" }),
		mutate(e, func(c *Channel) { c.From = "" }),
		mutate(e, func(c *Channel) { c.Subject = "a\r\nBcc: x@y" }),
		mutate(e, func(c *Channel) { c.Addressing = AddressingFixed }),
		mutate(e, func(c *Channel) { c.External = ptr(false) }),
		mutate(e, func(c *Channel) { c.AuthEnv = "not a name" }),
		mutate(e, func(c *Channel) { c.AddressPattern = "forensic" }),
		mutate(s, func(c *Channel) { c.Target = "http://gw.example/send" }),
		mutate(s, func(c *Channel) { c.BodyTemplate = `{"to":"{to}"}` }),
		mutate(s, func(c *Channel) { c.BodyTemplate = `to={to}&text={message}` }),
		mutate(s, func(c *Channel) { c.BodyShape = BodyShapeText }),
		{Name: "h", Type: TypeHTTP, Target: "https://x", Addressing: AddressingPerRecipient, AddressPattern: "^x$"},
		{Name: "h", Type: TypeHTTP, Target: "https://x", BodyShape: "adaptive-card"},
		{Name: "h", Type: TypeHTTP, Target: "https://x", AuthEnv: "X"},
		{Name: "f", Type: TypeFile, Target: "/tmp/x", From: "a@b.example"},
	}
	for i, ch := range bad {
		if err := Validate(Config{Channels: []Channel{ch}}); err == nil {
			t.Fatalf("bad channel %d accepted: %+v", i, ch)
		}
	}
}

func TestValidateDeliveriesRefuseDoubleSendWithoutNamingAddress(t *testing.T) {
	bad := []Request{
		{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "a"}, {Channel: "mail", To: strings.ToUpper(priya), Ref: "b"}}},
		{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "a"}, {Channel: "mail", To: "other@forensic.example", Ref: "a"}}},
		{Message: "x", Channels: []string{"local"}, Deliveries: []Delivery{{Channel: "local", Ref: "a"}}},
		{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya}}},
		{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "has space"}}},
		{Message: "x", Deliveries: []Delivery{{To: priya, Ref: "a"}}},
		{Message: "x", Deliveries: []Delivery{{Channel: "mail", To: strings.Repeat("a", MaxAddressLength+1), Ref: "a"}}},
	}
	for i, r := range bad {
		err := ValidateRequest(r)
		if err == nil {
			t.Fatalf("bad request %d accepted", i)
		}
		if strings.Contains(strings.ToLower(err.Error()), "priya@") {
			t.Fatalf("refusal %d names the address: %v", i, err)
		}
	}
	ok := Request{Message: "x", Channels: []string{"local"}, Deliveries: []Delivery{{Channel: "mail", To: priya, Ref: "priya.n"}, {Channel: "sms", To: tom, Ref: "priya.n"}}}
	if err := ValidateRequest(ok); err != nil {
		t.Fatal(err)
	}
}

func TestScrubIsCaseInsensitive(t *testing.T) {
	if got := scrub("rejected <PRIYA@Forensic.example> twice priya@forensic.example", priya); strings.Contains(strings.ToLower(got), "priya") {
		t.Fatal(got)
	}
}
