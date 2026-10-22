package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLocalAndHTTPOutcomes(t *testing.T) {
	p := t.TempDir() + "/alerts"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	c := Config{Channels: []Channel{{Name: "local", Type: "file", Target: p}, {Name: "external", Type: "http", Target: srv.URL, External: true}}}
	r := Service{}.Send(context.Background(), c, Request{Message: "break glass", Channels: []string{"local", "external"}, TimeoutMS: 1000})
	if r.AllSucceeded || r.Outcomes[0].Status != "delivered" || r.Outcomes[1].Status != "failed" {
		t.Fatalf("%+v", r)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "break glass") {
		t.Fatal("local missing")
	}
}

func TestExerciseSuppressesExternalBeforeIO(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	c := Config{Channels: []Channel{{Name: "local", Type: "file", Target: t.TempDir() + "/alerts"}, {Name: "x", Type: "http", Target: srv.URL, External: true}}}
	r := Service{}.Send(context.Background(), c, Request{Message: "exercise", Channels: []string{"local", "x"}, Exercise: true, TimeoutMS: 1000})
	if called || len(r.Outcomes) != 2 || r.Outcomes[0].Status != "delivered" || r.Outcomes[1].Status != "suppressed" || !r.AllSucceeded {
		t.Fatalf("%+v", r)
	}
}

func TestHTTPCannotMasqueradeAsLocal(t *testing.T) {
	err := Validate(Config{Channels: []Channel{{Name: "x", Type: "http", Target: "https://example.invalid/hook", External: false}}})
	if err == nil {
		t.Fatal("HTTP channel accepted as non-external")
	}
}

func TestHTTPRedirectIsNotFollowed(t *testing.T) {
	redirected := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer destination.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	r := Service{}.Send(context.Background(), Config{Channels: []Channel{{Name: "x", Type: "http", Target: source.URL, External: true}}}, Request{Message: "sensitive", Channels: []string{"x"}, TimeoutMS: 1000})
	if redirected {
		t.Fatal("HTTP notification followed redirect")
	}
	if len(r.Outcomes) != 1 || r.Outcomes[0].Status != "failed" || !strings.Contains(r.Outcomes[0].Detail, "307") {
		t.Fatalf("redirect was not reported as failed: %+v", r)
	}
}

func TestTimeoutVisible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(50 * time.Millisecond) }))
	defer srv.Close()
	r := Service{}.Send(context.Background(), Config{Channels: []Channel{{Name: "x", Type: "http", Target: srv.URL, External: true}}}, Request{Message: "x", Channels: []string{"x"}, TimeoutMS: 5})
	if r.Outcomes[0].Status != "failed" || !strings.Contains(r.Outcomes[0].Detail, "deadline exceeded") {
		t.Fatalf("%+v", r)
	}
}

func TestOneFailureDoesNotHideLaterOutcome(t *testing.T) {
	p := t.TempDir() + "/alerts"
	c := Config{Channels: []Channel{{Name: "local", Type: "file", Target: p}}}
	r := Service{}.Send(context.Background(), c, Request{Message: "x", Channels: []string{"missing", "local"}, TimeoutMS: 1000})
	if len(r.Outcomes) != 2 || r.Outcomes[0].Status != "failed" || r.Outcomes[1].Status != "delivered" || r.AllSucceeded {
		t.Fatalf("%+v", r)
	}
}

func TestValidateConfigAndRequest(t *testing.T) {
	valid := Config{Channels: []Channel{{Name: "local", Type: "file", Target: "/tmp/x"}, {Name: "ext", Type: "http", Target: "https://example.invalid/hook", External: true}}}
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	badConfigs := []Config{
		{},
		{Channels: []Channel{{Name: "", Type: "file", Target: "/tmp/x"}}},
		{Channels: []Channel{{Name: "x", Type: "file", Target: ""}}},
		{Channels: []Channel{{Name: "x", Type: "smtp", Target: "x"}}},
		{Channels: []Channel{{Name: "x", Type: "file", Target: "/tmp/x", External: true}}},
		{Channels: []Channel{{Name: "x", Type: "http", Target: "https://example.invalid", External: false}}},
		{Channels: []Channel{{Name: "x", Type: "file", Target: "/tmp/a"}, {Name: "x", Type: "file", Target: "/tmp/b"}}},
	}
	for i, c := range badConfigs {
		if err := Validate(c); err == nil {
			t.Fatalf("bad config %d accepted", i)
		}
	}
	goodReq := Request{Message: "x", Channels: []string{"local"}}
	if err := ValidateRequest(goodReq); err != nil {
		t.Fatal(err)
	}
	badReqs := []Request{
		{},
		{Message: "x"},
		{Message: "x", Channels: []string{""}},
		{Message: "x", Channels: []string{"local", "local"}},
		{Message: "x", Channels: []string{"local"}, TimeoutMS: -1},
		{Message: "x", Channels: []string{"local"}, TimeoutMS: MaxTimeoutMS + 1},
	}
	for i, r := range badReqs {
		if err := ValidateRequest(r); err == nil {
			t.Fatalf("bad request %d accepted", i)
		}
	}
}
