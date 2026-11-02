# Notifier — Generated Engineering Design Document

**Evidence commit:** `ad9ddf1c8dc1974dae2d7dc8e51dbc23c1701e7d`  
**Method:** code-derived review using the `codebase-to-edd` evidence discipline  
**Generated:** 2026-11-02

This document describes the implementation at the evidence commit. The hand-written PRD/EDD are comparison targets, not runtime evidence.

## 1. Executive description

**KNOWN** — `notifier` is a small synchronous Go component with one public package, `notify` (`notify.go`, `validate.go`, `http.go`, `email.go`), and a thin JSON-file CLI.

**KNOWN** — it attempts an already-composed message through configured channels, to fixed targets or to caller-supplied addresses, and emits one ordered `delivered`, `failed` or `suppressed` outcome per request entry, identified by channel and caller `ref`.

## 2. Evidence inspected

- `notify/notify.go` — types, work list, address checks, exercise suppression, file adapter, scrub;
- `notify/validate.go` — configuration and request contracts;
- `notify/http.go` — http and sms-http adapters, template rendering;
- `notify/email.go` — SMTP adapter, STARTTLS, error reporting, message composition;
- `notify/*_test.go` — behavioural evidence, local SMTP (STARTTLS) and HTTPS servers;
- `cmd/notifier/main.go`, `main_test.go` — process and exit contract, address-free output;
- `.github/workflows/test.yml` — engineering gate;
- `examples/bafflewick/` — offline acceptance paths;
- `go.mod`, `VERSION`, `CHANGELOG.md` — language/release boundary.

## 3. Runtime structure

```text
config + request
      |
      v
Validate + ValidateRequest
      |
      v
work list: request.channels, then request.deliveries
      |
      +-- unknown channel -> failed
      +-- addressing / pattern / mailbox check fails -> failed (no I/O)
      +-- exercise + non-file type -> suppressed (no I/O)
      +-- file -> append
      +-- http -> context-bound POST, message|text body
      +-- sms-http -> https check, credentials, rendered JSON POST
      +-- email -> credentials, dial, STARTTLS, AUTH, MAIL, RCPT, DATA
      |
      v
detail scrubbed of address; Outcome{channel, ref, status, detail, duration}
      |
      v
Result{Outcomes, AllSucceeded}
```

## 4. Configuration contract

**KNOWN** — at least one channel; names and targets non-blank; names unique.

**KNOWN** — supported types are exactly `file`, `http`, `email`, `sms-http`. `IsExternal` returns false only for `file`, so an unknown type is treated as external.

**KNOWN** — `external` is a `*bool`: absent is accepted; present must agree with the type.

**KNOWN** — `addressing` is derived from the type (email and sms-http per_recipient, others fixed) and, if present, must match.

**KNOWN** — per-recipient channels require an `address_pattern` that compiles and starts with `^` and ends with `$`; fixed channels may not carry one.

**KNOWN** — type-specific fields are refused on other types (`body_template` sms-http only; `body_shape` http only; `from`/`subject` email only; `auth_env` email and sms-http only, and must look like an environment variable name).

**KNOWN** — email requires a `host:port` target, a bare `from` mailbox and a one-line subject; sms-http requires an https target (http only for localhost/127.0.0.1/::1) and a template containing `{to}` and `{message}` that renders to valid JSON with a sample.

## 5. Request contract

**KNOWN** — message non-blank; at least one channel or delivery; timeout 0 (3000 ms default) or 1–60000.

**KNOWN** — requested channel names non-blank and unique.

**KNOWN** — each delivery needs a channel and a `ref` (no whitespace or control characters, at most 256); `to` at most 320; `ref` unique per channel.

**KNOWN** — a single destination key set covers both lists: `channel` alone for fixed sends and `channel + lower(to)` for addressed sends. A repeat is refused, so a fixed channel listed in `channels` and again as a delivery, or the same address twice, cannot double-send.

**KNOWN** — refusal messages contain delivery numbers, refs and channel names; none interpolate `to`.

## 6. Address checks

**KNOWN** — run before exercise suppression and before any context, connection or request is created.

**KNOWN** — fixed channel with an address → `fixed channel takes no address`; per-recipient with no address → `address required`; whitespace/control characters, over 320, pattern mismatch, or (email) not a bare `net/mail` mailbox → `address refused`.

**KNOWN** — `Send` recompiles patterns itself, so a channel whose pattern is empty, unanchored or invalid refuses every address even if `Validate` was skipped (`TestUnvalidatedPerRecipientChannelIsNeverOpen`).

## 7. Exercise safety

**KNOWN** — every non-file channel is suppressed in exercise mode before I/O; file channels still deliver; suppression does not clear `AllSucceeded`.

## 8. File adapter

**KNOWN** — unchanged from v0.1: cancellation checked before open; create/append/write, mode `0600`; message plus newline.

## 9. HTTP and sms-http adapters

**KNOWN** — shared `post`: JSON POST to the configured target, optional `Authorization`, redirects not followed, up to 64 KiB drained, 2xx delivered, otherwise `HTTP status N`. Response bodies are never read into outcomes.

**KNOWN** — http body is `{"message":…}` by default or `{"text":…}` with `body_shape: "text"`.

**KNOWN** — sms-http refuses a non-https target and a missing `auth_env` value before I/O, renders the template with a single-pass `strings.Replacer` of JSON-escaped values, and refuses a render that is not valid JSON.

## 10. Email adapter

**KNOWN** — credentials (`user:password`) are read before dialling; absent or malformed credentials fail with nothing sent.

**KNOWN** — dial uses the delivery context; the socket deadline is the context deadline; `context.AfterFunc` closes the connection when the context ends.

**KNOWN** — without `STARTTLS` in EHLO the delivery fails before AUTH/MAIL/RCPT/DATA. TLS uses the service config clone, ServerName defaulting to the target host, minimum TLS 1.2.

**KNOWN** — with credentials, `AUTH` must be offered; PLAIN is used after TLS.

**KNOWN** — `smtpError` reports `smtp <stage> refused (<code>)` for server replies, maps context and socket-deadline errors to the context error, and for recipient/data stages never includes error text. Other stages (connect, greeting, starttls, authentication) may include Go error text, which contains server addresses, not recipients.

**KNOWN** — message: From, To, Q-encoded Subject (default `Notification`), Date (host clock), MIME text/plain UTF-8, quoted-printable body with CRLF line endings; dot-stuffing by `net/smtp`'s data writer.

## 11. Address containment

**KNOWN** — `Outcome` has no address field. Every detail passes through a case-insensitive `scrub` replacing the address with `[recipient]`.

**KNOWN** — the CLI prints outcomes and validation errors; a request decode failure prints the file name only.

## 12. Timeout model

**KNOWN** — per attempted delivery, child context of the caller's. **INFERRED** — worst-case wall time approaches deliveries × timeout.

## 13. Failure isolation

**KNOWN** — every failure path returns an outcome and the loop continues. No retry, backoff or failover.

## 14. CLI contract

**KNOWN** — `-version` prints `notifier 0.2.0` (CI compares against `VERSION`). Exit codes 0/1/2/3 unchanged from v0.1. `run(args, stdout, stderr)` holds the whole CLI.

## 15. State and dependencies

**KNOWN** — no queue, database, durable state or background worker. `go.mod` targets Go 1.26 with no third-party modules.

## 16. Security boundary

**KNOWN** — configuration trusted; requests choose addresses within each channel's anchored pattern; credentials only from the environment; email addresses must be bare mailboxes, so CR/LF header injection is refused (`TestPatternRefusalBeforeIO`).

## 17. Executable evidence

v0.1 tests (unchanged in behaviour): `TestLocalAndHTTPOutcomes`, `TestExerciseSuppressesExternalBeforeIO`, `TestHTTPCannotMasqueradeAsLocal`, `TestHTTPRedirectIsNotFollowed`, `TestTimeoutVisible`, `TestOneFailureDoesNotHideLaterOutcome`, `TestValidateConfigAndRequest`.

v0.2 tests: `TestPerRecipientEmailOverSTARTTLS`, `TestEmailRequiresSTARTTLS`, `TestEmailUntrustedCertificateFails`, `TestEmailRejectionNeverEchoesAddress`, `TestEmailBadCredentialsAndMissingCredentials`, `TestPerRecipientSMSHTTP`, `TestHTTPTextBodyShape`, `TestPatternRefusalBeforeIO`, `TestUnvalidatedPerRecipientChannelIsNeverOpen`, `TestUnvalidatedSMSChannelNeverSendsInPlaintext`, `TestExerciseSuppressesEveryExternalTypeBeforeIO`, `TestMixedFixedAndPerRecipient`, `TestV01RequestAndConfigCompatibility`, `TestV01ChannelNameWhitespaceStillAccepted`, `TestTimeoutsBoundedPerDelivery`, `TestValidatePerRecipientConfig`, `TestValidateDeliveriesRefuseDoubleSendWithoutNamingAddress`, `TestScrubIsCaseInsensitive`, `TestCLINeverPrintsAddresses`, `TestCLIVersionAndUsage`.

All servers are local: a STARTTLS SMTP server with a generated certificate, `httptest` HTTPS and HTTP servers.

## 18. Bafflewick paths

**KNOWN** — the v0.1 request writes `Bafflewick break-glass activated` to the local sink offline.

**KNOWN** — the per-recipient exercise request delivers locally, suppresses webhook, email and SMS, and refuses an out-of-pattern address, offline, exit 1, with no address in output (CI step).

## 19. Known limits / unknowns

**KNOWN** — no guaranteed delivery, receipt, retry queue, recipient directory, address store or vendor SDK; AUTH PLAIN over STARTTLS only (no implicit TLS); EHLO name is `localhost`; Date header from the host clock; deliveries sequential.

**KNOWN** — filesystem cancellation checked before I/O only.

**UNKNOWN** — behaviour against specific production relays and gateways; the tests use local protocol-faithful servers.

**UNKNOWN** — runtime performance on the Garage constrained-hardware target; ARM64 CI is compilation proof only.

## 20. Evidence conclusion

**KNOWN** — the source implements the per-recipient boundary CRAB requires (`docs/notifier-v0.2-requirement.md` in CRAB): addressing per delivery, anchored allow-patterns before I/O, `ref` not `to` in outcomes, email/sms-http/text-webhook channels without SDKs, and the v0.1 guarantees. The safety boundaries are executable in tests.
