package notify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"io"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testSMTP is a minimal SMTP submission server for component acceptance.
type testSMTP struct {
	t        *testing.T
	ln       net.Listener
	tlsCfg   *tls.Config
	roots    *x509.CertPool
	offerTLS bool
	user     string
	pass     string
	// rejectRcpt makes RCPT fail with a reply that echoes the address, as
	// real servers do.
	rejectRcpt bool
	hang       bool

	conns atomic.Int32
	mu    sync.Mutex
	lines []string // commands seen, in order
	tls   []bool   // whether each command arrived over TLS
	data  []string // message bodies accepted
	rcpts []string
}

func newTestSMTP(t *testing.T, configure func(*testSMTP)) *testSMTP {
	t.Helper()
	s := &testSMTP{t: t, offerTLS: true}
	if configure != nil {
		configure(s)
	}
	s.tlsCfg, s.roots = selfSigned(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.ln = ln
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.conns.Add(1)
			go s.serve(c)
		}
	}()
	return s
}

func (s *testSMTP) addr() string { return s.ln.Addr().String() }

func (s *testSMTP) service() Service {
	return Service{TLSConfig: &tls.Config{RootCAs: s.roots}, Getenv: func(name string) string {
		if name == "TEST_SMTP_AUTH" && s.user != "" {
			return s.user + ":" + s.pass
		}
		return ""
	}}
}

func (s *testSMTP) record(line string, overTLS bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, line)
	s.tls = append(s.tls, overTLS)
}

func (s *testSMTP) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}

// snapshot copies everything the server recorded, under its lock.
func (s *testSMTP) snapshot() (lines []string, overTLS []bool, rcpts, data []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...), append([]bool(nil), s.tls...), append([]string(nil), s.rcpts...), append([]string(nil), s.data...)
}

func (s *testSMTP) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if s.hang {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	tp := textproto.NewConn(conn)
	overTLS := false
	reply := func(lines ...string) {
		for _, l := range lines {
			_ = tp.PrintfLine("%s", l)
		}
	}
	reply("220 test.invalid ESMTP")
	for {
		line, err := tp.ReadLine()
		if err != nil {
			return
		}
		s.record(line, overTLS)
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		switch verb {
		case "EHLO":
			switch {
			case !overTLS && s.offerTLS:
				reply("250-test.invalid", "250-8BITMIME", "250 STARTTLS")
			case overTLS:
				reply("250-test.invalid", "250 AUTH PLAIN")
			default:
				reply("250-test.invalid", "250 AUTH PLAIN")
			}
		case "HELO":
			reply("250 test.invalid")
		case "STARTTLS":
			reply("220 ready")
			tc := tls.Server(conn, s.tlsCfg)
			if err := tc.Handshake(); err != nil {
				return
			}
			conn = tc
			tp = textproto.NewConn(tc)
			overTLS = true
		case "AUTH":
			fields := strings.Fields(line)
			want := base64.StdEncoding.EncodeToString([]byte("\x00" + s.user + "\x00" + s.pass))
			if len(fields) == 3 && fields[2] == want {
				reply("235 2.7.0 ok")
			} else {
				reply("535 5.7.8 authentication failed")
			}
		case "MAIL":
			reply("250 2.1.0 ok")
		case "RCPT":
			addr := strings.TrimSuffix(strings.TrimPrefix(line[len("RCPT TO:"):], "<"), ">")
			if s.rejectRcpt {
				reply("550 5.1.1 <" + addr + ">: Recipient address rejected: user unknown")
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, addr)
			s.mu.Unlock()
			reply("250 2.1.5 ok")
		case "DATA":
			reply("354 go ahead")
			b, err := tp.ReadDotBytes()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.data = append(s.data, string(b))
			s.mu.Unlock()
			reply("250 2.0.0 queued")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func selfSigned(t *testing.T) (*tls.Config, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "notifier test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, pool
}
