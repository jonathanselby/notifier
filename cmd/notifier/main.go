package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jonathanselby/notifier/notify"
)

const version = "0.2.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the whole CLI. It never prints a delivery address: outcomes carry
// refs, and refusals name channels and refs only.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("notifier", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var cfg, req string
	var v bool
	fs.StringVar(&cfg, "config", "", "channel config JSON")
	fs.StringVar(&req, "request", "", "notification request JSON")
	fs.BoolVar(&v, "version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if v {
		fmt.Fprintln(stdout, "notifier "+version)
		return 0
	}
	if cfg == "" || req == "" {
		fs.Usage()
		return 2
	}
	var c notify.Config
	var r notify.Request
	if err := read(cfg, &c); err != nil {
		fmt.Fprintln(stderr, "config:", err)
		return 3
	}
	if err := read(req, &r); err != nil {
		// Decoding errors can quote input; report the file only.
		fmt.Fprintln(stderr, "request: cannot read or decode", req)
		return 3
	}
	if err := notify.Validate(c); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := notify.ValidateRequest(r); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	out := notify.Service{}.Send(context.Background(), c, r)
	if err := json.NewEncoder(stdout).Encode(out); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	if !out.AllSucceeded {
		return 1
	}
	return 0
}

func read(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
