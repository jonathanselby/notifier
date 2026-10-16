package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/jonathanselby/notifier/internal/notify"
)

const version = "0.1.0"

func main() {
	var cfg, req string
	var v bool
	flag.StringVar(&cfg, "config", "", "channel config JSON")
	flag.StringVar(&req, "request", "", "notification request JSON")
	flag.BoolVar(&v, "version", false, "print version")
	flag.Parse()
	if v {
		fmt.Println("notifier " + version)
		return
	}
	if cfg == "" || req == "" {
		flag.Usage()
		os.Exit(2)
	}
	var c notify.Config
	var r notify.Request
	if err := read(cfg, &c); err != nil {
		fatal(err)
	}
	if err := read(req, &r); err != nil {
		fatal(err)
	}
	if err := notify.Validate(c); err != nil {
		refuse(err)
	}
	if err := notify.ValidateRequest(r); err != nil {
		refuse(err)
	}
	out := notify.Service{}.Send(context.Background(), c, r)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fatal(err)
	}
	if !out.AllSucceeded {
		os.Exit(1)
	}
}

func read(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func refuse(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(3)
}
