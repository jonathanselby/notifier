# Changelog

## 0.2.0 — 2026-11-02

- public package `github.com/jonathanselby/notifier/notify` (was `internal/notify`), so applications can import it;
- per-recipient addressing: requests may carry `deliveries` of `{channel, to, ref}`; outcomes echo `ref`, never the address;
- anchored `address_pattern` per per-recipient channel; a non-matching address is refused before any I/O;
- `email` channel: SMTP submission, STARTTLS required, AUTH PLAIN with credentials named by environment variable, fixed sender, quoted-printable text body;
- `sms-http` channel: HTTPS POST with a JSON body template (`{to}`, `{message}`), optional `Authorization` from the environment;
- `http` channel: optional `body_shape: "text"` for Slack/Teams incoming webhooks;
- external is derived from type; v0.1 `external` metadata is still accepted when it agrees;
- server refusal text is never copied into outcomes; details are scrubbed of the address;
- duplicate destinations in one request are refused;
- CLI never prints addresses; v0.1 configuration, requests and outcome shape unchanged.

## 0.1.0 — 2026-10-17

- initial local-file and HTTP notification component;
- explicit delivered/failed/suppressed outcomes;
- exercise suppression before HTTP I/O;
- HTTP channels validated as external and classified by type at runtime;
- per-channel timeout default/bounds;
- duplicate-request refusal to prevent accidental double-send;
- deterministic config/request validation and exit-code separation;
- channel-failure isolation with no automatic retry;
- Go 1.26 standard-library-only baseline;
- PRD, EDD, architecture, skill, release discipline, Bafflewick acceptance and code-derived evidence.
