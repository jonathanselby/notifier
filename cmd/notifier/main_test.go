package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

const perRecipientConfig = `{"channels":[
 {"name":"local","type":"file","target":"%FILE%"},
 {"name":"mail","type":"email","target":"127.0.0.1:1","addressing":"per_recipient","address_pattern":"^[^@\\s]+@forensic\\.example$","from":"crab@forensic.example"},
 {"name":"sms","type":"sms-http","target":"https://127.0.0.1:1/send","addressing":"per_recipient","address_pattern":"^\\+[1-9][0-9]{7,14}$","body_template":"{\"to\":\"{to}\",\"text\":\"{message}\"}"}]}`

func TestCLINeverPrintsAddresses(t *testing.T) {
	cfg := write(t, "config.json", strings.ReplaceAll(perRecipientConfig, "%FILE%", filepath.Join(t.TempDir(), "alerts")))
	cases := []struct {
		name, request string
		code          int
	}{
		{"exercise with a refused address", `{"message":"x","exercise":true,"channels":["local"],"deliveries":[{"channel":"mail","to":"priya@forensic.example","ref":"priya.n"},{"channel":"mail","to":"mallory@elsewhere.example","ref":"m"},{"channel":"sms","to":"+61400000789","ref":"tom.b"}]}`, 1},
		{"duplicate destination", `{"message":"x","deliveries":[{"channel":"mail","to":"priya@forensic.example","ref":"a"},{"channel":"mail","to":"priya@forensic.example","ref":"b"}]}`, 1},
		{"unreachable server", `{"message":"x","timeout_ms":500,"deliveries":[{"channel":"mail","to":"priya@forensic.example","ref":"priya.n"},{"channel":"sms","to":"+61400000789","ref":"tom.b"}]}`, 1},
		{"malformed request", `{"message":"x","deliveries":[{"channel":"mail","to":"priya@forensic.example","ref":`, 3},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := run([]string{"-config", cfg, "-request", write(t, "req.json", tc.request)}, &out, &errOut)
		if code != tc.code {
			t.Fatalf("%s: exit %d, want %d\nstdout %s\nstderr %s", tc.name, code, tc.code, out.String(), errOut.String())
		}
		all := out.String() + errOut.String()
		for _, addr := range []string{"priya@", "mallory", "+61400000789"} {
			if strings.Contains(all, addr) {
				t.Fatalf("%s: %q printed:\n%s", tc.name, addr, all)
			}
		}
	}
}

func TestCLIVersionAndUsage(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"-version"}, &out, &errOut); code != 0 || out.String() != "notifier 0.2.0\n" {
		t.Fatalf("%d %q", code, out.String())
	}
	if code := run(nil, &out, &errOut); code != 2 {
		t.Fatalf("usage exit %d", code)
	}
}
