# Notifier

[![test](https://github.com/jonathanselby/notifier/actions/workflows/test.yml/badge.svg)](https://github.com/jonathanselby/notifier/actions/workflows/test.yml)

**Delivery is an outcome to record, not an assumption to hide.**

`notifier` is a deliberately small Go component for attempting notification delivery through configured channels (local file, webhook, email, SMS gateway) while recording the outcome of every attempt. It is usable independently of any larger application, as a Go package or a CLI.

## The job

Given an already composed message and an explicit ordered list of channels and deliveries:

1. validate the channel configuration and request;
2. check each per-recipient address against the channel's allow-pattern, before any I/O;
3. suppress every external delivery before I/O when the request is an exercise;
4. attempt each remaining delivery independently within its time budget;
5. record `delivered`, `failed` or `suppressed` for every requested delivery, identified by the caller's `ref`, never by the address;
6. never retry or hide a failure.

## Channel boundary

| Type | External | Addressing | Target |
|---|---|---|---|
| `file` | never | fixed | path; local append sink |
| `http` | always | fixed | URL; JSON POST `{"message":…}`, or `{"text":…}` with `"body_shape":"text"` for Slack/Teams incoming webhooks |
| `email` | always | per_recipient | `host:port`; SMTP submission, **STARTTLS required**, fixed `from`, optional `subject`, optional `auth_env` holding `user:password` |
| `sms-http` | always | per_recipient | `https://` URL; JSON POST rendered from `body_template` with `{to}` and `{message}`, optional `auth_env` holding the `Authorization` value |

External is derived from the type. The v0.1 `external` field is still accepted when it agrees with the type, and refused when it does not.

Per-recipient channels must declare an `address_pattern` anchored with `^` and `$`. An address that does not match is refused as `failed` with detail `address refused` before any connection is made, so the component cannot become an open relay. Email addresses must also be bare mailboxes (no display names, spaces or line breaks).

## What this does not do

It does not compose messages, choose or store recipients, debounce, integrate vendor SDKs, queue retries, decide escalation, guarantee delivery or interpret a 2xx response as end-recipient receipt.

## Use as a Go package

```go
import "github.com/jonathanselby/notifier/notify"

if err := notify.Validate(cfg); err != nil { /* refuse config */ }
if err := notify.ValidateRequest(req); err != nil { /* refuse request */ }
result := notify.Service{}.Send(ctx, cfg, req)
```

## Install

Requires Go 1.26.

```bash
go install github.com/jonathanselby/notifier/cmd/notifier@main
notifier -version
```

Expected version: `notifier 0.2.0`.

## Run

```bash
notifier -config examples/bafflewick/config.json -request examples/bafflewick/request.json
```

The Bafflewick fixture uses only the local sink and therefore needs no network.

Per-recipient example (an exercise, so email and SMS are suppressed and no network is needed; one address is deliberately refused):

```bash
notifier -config examples/bafflewick/per-recipient-config.json -request examples/bafflewick/per-recipient-exercise.json
```

A per-recipient request:

```json
{"message": "INC-007: 6 items are waiting for your decision. Sign in to CRAB.",
 "timeout_ms": 3000,
 "channels": ["local-major-users"],
 "deliveries": [
   {"channel": "mail", "to": "priya@bafflewick.example", "ref": "priya.n"},
   {"channel": "sms",  "to": "+61400000789",             "ref": "tom.b"}
 ]}
```

Outcomes follow `channels` first, then `deliveries`, in order.

## Result

```json
{
  "outcomes": [
    {"channel":"local-major-users","status":"delivered","duration_ms":0},
    {"channel":"mail","ref":"priya.n","status":"delivered","duration_ms":412},
    {"channel":"sms","ref":"tom.b","status":"failed","detail":"HTTP status 502","duration_ms":96}
  ],
  "all_succeeded": false
}
```

The address never appears in outcomes, refusals or CLI output. SMTP refusals are reported by stage and reply code (for example `smtp recipient refused (550)`), because servers echo the recipient in their reply text.

`all_succeeded` remains true for deliberately suppressed exercise channels; suppression is an intentional outcome, not a delivery failure.

## Stable exit codes

| Code | Meaning |
|---:|---|
| `0` | all attempted deliveries delivered or were deliberately suppressed |
| `1` | deterministic config/request refusal or one/more delivery failures |
| `2` | invalid CLI usage |
| `3` | malformed JSON, file I/O or output execution failure |

## Timeout semantics

`timeout_ms` is per attempted delivery. Zero means the 3000 ms default; supplied values must be 1–60000 ms.

HTTP and SMTP are context-cancellable. The local file sink checks cancellation before opening the file, but Go cannot reliably interrupt an already-blocking filesystem syscall; the component does not claim a hard wall-clock deadline for a wedged filesystem.

## Security boundary

Configuration is trusted deployment input. Targets are never discovered from messages, and the component does not authorise recipients: the consuming application decides who may be notified. Each per-recipient channel's anchored pattern is the relay boundary for caller-supplied addresses. Credentials are read from the environment variable named by `auth_env`, never from configuration or requests. Webhook secrets embedded in `http` target URLs remain the deployment's responsibility.

## Engineering artefacts

- `PRD.md` — product boundary and testable acceptance criteria;
- `EDD.md` — hand-written design and operational boundary;
- `architecture.txt` — plain-text runtime view;
- `notify` — reusable core (public Go package);
- `cmd/notifier` — thin CLI edge;
- `examples/bafflewick` — fictional acceptance case;
- `.github/workflows/test.yml` — Go 1.26 format/vet/race/build/version/ARM64/acceptance gate, including the offline per-recipient exercise;
- `generated/EDD.md` — code-derived design evidence pinned to a source commit;
- `generated/COMPARISON.md` — intended design versus recovered implementation;
- `SKILL.md`, `RELEASE.md`, `CHANGELOG.md`, `VERSION` — adoption/release discipline.

## Licence

MIT. See `LICENSE`.
