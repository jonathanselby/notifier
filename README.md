# Notifier

[![test](https://github.com/jonathanselby/notifier/actions/workflows/test.yml/badge.svg)](https://github.com/jonathanselby/notifier/actions/workflows/test.yml)

**Delivery is an outcome to record, not an assumption to hide.**

`notifier` is a deliberately small Go component for attempting notification delivery through configured local and HTTP channels while recording the outcome of every attempt. It is usable independently of any larger application.

## The job

Given an already composed message and an explicit ordered list of channels:

1. validate the channel configuration and request;
2. suppress HTTP delivery before I/O when the request is an exercise;
3. attempt each remaining channel independently within its per-channel time budget;
4. record `delivered`, `failed` or `suppressed` for every requested channel;
5. never retry or hide a failed channel.

## Channel boundary

V0.1 supports two channel types:

- `file` — local append sink; must not be marked external;
- `http` — HTTP POST webhook; **must** be marked external and is always treated as external by the runtime.

The `external` field is therefore validated metadata, not a caller-controlled way to bypass exercise suppression.

## What this does not do

It does not compose messages, own recipients, integrate email/SMS vendor SDKs, queue retries, decide escalation, guarantee delivery or interpret a 2xx response as end-recipient receipt.

## Install

Requires Go 1.26.

```bash
go install github.com/jonathanselby/notifier/cmd/notifier@main
notifier -version
```

Expected version: `notifier 0.1.0`.

## Run

```bash
notifier -config examples/bafflewick/config.json -request examples/bafflewick/request.json
```

The Bafflewick fixture uses only the local sink and therefore needs no network.

## Result

```json
{
  "outcomes": [
    {"channel":"local-major-users","status":"delivered","duration_ms":0}
  ],
  "all_succeeded": true
}
```

`all_succeeded` remains true for deliberately suppressed exercise channels; suppression is an intentional outcome, not a delivery failure.

## Stable exit codes

| Code | Meaning |
|---:|---|
| `0` | all attempted channels delivered or were deliberately suppressed |
| `1` | deterministic config/request refusal or one/more channel failures |
| `2` | invalid CLI usage |
| `3` | malformed JSON, file I/O or output execution failure |

## Timeout semantics

`timeout_ms` is per attempted channel. Zero means the 3000 ms default; supplied values must be 1–60000 ms.

HTTP is context-cancellable. The local file sink checks cancellation before opening the file, but Go cannot reliably interrupt an already-blocking filesystem syscall; V0.1 does not claim a hard wall-clock deadline for a wedged filesystem.

## Security boundary

Configuration is trusted deployment input. HTTP targets are not discovered from notification messages and the component does not perform recipient authorisation. A consuming application must protect configuration from untrusted modification and own any webhook authentication/secret handling outside this V0.1 adapter.

## Engineering artefacts

- `PRD.md` — product boundary and testable acceptance criteria;
- `EDD.md` — hand-written design and operational boundary;
- `architecture.txt` — plain-text runtime view;
- `internal/notify` — reusable core;
- `cmd/notifier` — thin CLI edge;
- `examples/bafflewick` — fictional acceptance case;
- `.github/workflows/test.yml` — Go 1.26 format/vet/race/build/version/ARM64/acceptance gate;
- `generated/EDD.md` — code-derived design evidence pinned to a source commit;
- `generated/COMPARISON.md` — intended design versus recovered implementation;
- `SKILL.md`, `RELEASE.md`, `CHANGELOG.md`, `VERSION` — adoption/release discipline.

## Licence

MIT. See `LICENSE`.
