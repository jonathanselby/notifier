# Notifier — Engineering Design Document

**Version:** 0.2

## 1. Purpose

Attempt already-composed notifications through explicit channels, to fixed targets or to per-delivery addresses each channel allows, and return one visible outcome for every requested delivery.

> **Delivery is an outcome to record, not an assumption to hide.**

## 2. Design constraints

1. Local operation must not depend on external channels.
2. Exercise mode must suppress external I/O before the adapter runs.
3. A failed delivery must not stop later deliveries.
4. No automatic retry or hidden failover.
5. External is derived from the channel type, never from caller metadata.
6. Message and recipient policy remain upstream.
7. **An address is checked against the channel before any I/O, and never appears in an outcome, refusal or log.**
8. Standard library only; no vendor SDKs.

## 3. Runtime model

```text
config + request
      |
      v
Validate (types, addressing, anchored patterns, adapter fields)
ValidateRequest (message, channels, deliveries, refs, timeout, duplicates)
      |
      v
work list = channels (v0.1 form) then deliveries, each in order
      |
      +-- unknown channel --------------------> failed
      +-- address rule / pattern fails -------> failed: address refused | address required | fixed channel takes no address
      +-- exercise + external type -----------> suppressed (no I/O)
      +-- file ---------------------> append -> delivered | failed
      +-- http ---------------------> POST {"message"|"text"} -> delivered | failed
      +-- sms-http -----------------> POST rendered template -> delivered | failed
      +-- email --------------------> SMTP + STARTTLS -> delivered | failed
      |
      v
detail scrubbed of the address; outcome carries channel + ref
      |
      v
Result{Outcomes, AllSucceeded}
```

Address checks run before exercise suppression, so a rehearsal still reports a bad address.

## 4. Public package

`github.com/jonathanselby/notifier/notify` (moved from `internal/notify` so applications can import it):

- `Channel`, `Config`, `Delivery`, `Request`, `Outcome`, `Result`;
- `Validate(Config)`, `ValidateRequest(Request)`;
- `Service{Client, TLSConfig, Getenv}.Send(ctx, Config, Request) Result`;
- `IsExternal(type)` and the type, addressing, body-shape and status constants.

The zero `Service` uses a default HTTP client, system TLS roots and `os.Getenv`.

## 5. Channel types and addressing

| Type | External | Addressing | Target | Extra fields |
|---|---|---|---|---|
| `file` | never | fixed | path | — |
| `http` | always | fixed | URL | `body_shape`: `message` (default) or `text` |
| `email` | always | per_recipient | `host:port` | `address_pattern`, `from`, optional `subject`, optional `auth_env` |
| `sms-http` | always | per_recipient | `https://` URL | `address_pattern`, `body_template`, optional `auth_env` |

`addressing` may be omitted (it is derived from the type) and, when present, must match. A field that does not apply to the type is refused, so a typo cannot silently weaken a channel.

## 6. Configuration validation

`Validate` requires at least one channel; non-empty, unique, untrimmed names; non-empty targets; a supported type; and:

- `external`, when present (v0.1 config), must agree with the type;
- per-recipient channels need an `address_pattern` that compiles and is anchored with `^` and `$`;
- email: `host:port` target, bare `from` address, one-line subject;
- sms-http: `https` target (plain `http` only for loopback test endpoints), a template containing `{to}` and `{message}` that renders to valid JSON;
- `auth_env` is an environment variable *name*, allowed only on email and sms-http.

Errors name the channel, never an address.

## 7. Request validation

`ValidateRequest` requires a non-blank message; at least one channel or delivery; a timeout of zero (default) or 1–60000 ms; and:

- `channels`: non-empty, unique names;
- each delivery: a channel, a `ref` (no spaces or control characters, at most 256), an address of at most 320 characters;
- `ref` unique per channel;
- **one destination per request**: a fixed channel named twice (in `channels` and as a delivery), or the same channel and address (case-insensitive) twice, is refused rather than sent twice.

Refusals name the delivery number, ref and channel, never the address.

## 8. Address checks (before I/O)

For each work item on a per-recipient channel: an empty address is `address required`; an address with whitespace or control characters, longer than 320, not matching the pattern, or (email) not a bare `local@domain` mailbox is `address refused`. A per-recipient channel whose pattern is missing, unanchored or invalid refuses every address even if the caller skipped `Validate`. A fixed channel given an address is `fixed channel takes no address`.

## 9. Exercise suppression

For an exercise request, every external type (http, email, sms-http) is `suppressed` before a context, connection or request is created. File channels remain eligible. Suppression does not set `AllSucceeded=false`.

## 10. File adapter

Unchanged from v0.1: create/append/write, mode `0600` for new files, message plus newline. The context is checked before opening; an already-blocking filesystem call cannot be interrupted.

## 11. HTTP and sms-http adapters

Both POST JSON to the configured target only, with `Content-Type: application/json`, no redirect following, and up to 64 KiB of response drained and discarded. 2xx is `delivered` (adapter acceptance, not end-recipient receipt); anything else is `HTTP status N`. Response bodies are never copied into outcomes.

- `http` sends `{"message": …}` or, with `body_shape: "text"`, `{"text": …}`.
- `sms-http` renders `body_template` in one pass, substituting JSON-string-escaped values, so a message containing `{to}` is not expanded. With `auth_env`, the variable's value is sent as the `Authorization` header; a missing value fails the delivery before I/O.

## 12. Email adapter

1. Read credentials (`user:password` from `auth_env`), failing before I/O if absent.
2. Dial the target within the delivery context; the socket deadline is the context deadline, and the connection is closed if the context ends.
3. Require `STARTTLS` in the EHLO reply, else fail with nothing sent. Upgrade with the service TLS config (TLS 1.2 minimum, server name from the target host).
4. With credentials, require `AUTH` and authenticate with PLAIN over the encrypted connection.
5. `MAIL FROM` the configured sender, `RCPT TO` the address, `DATA` a `text/plain; charset=utf-8` quoted-printable message with `From`, `To`, `Subject` (Q-encoded) and `Date` headers. Line endings are normalised to CRLF; dot-stuffing is done by the SMTP writer.

Server refusals are reported as `smtp <stage> refused (<code>)`. Server reply text is never copied, because servers echo the recipient. As a final guard, every detail is scrubbed of the address (case-insensitive).

## 13. Timeout model

Timeout is per attempted delivery, not a whole-request deadline. Zero selects 3000 ms. Each attempt gets a child of the caller's context. Total time can approach deliveries × timeout.

## 14. Failure isolation

Unknown channels, address refusals, adapter errors, non-2xx, SMTP refusals and timeouts produce a failed outcome and set `AllSucceeded=false`; iteration continues. No retry or backoff.

## 15. CLI contract

```text
notifier -config <channels.json> -request <notification.json>
notifier -version
```

```text
0 all attempted deliveries delivered or were suppressed
1 semantic config/request refusal or one/more delivery failures
2 invalid CLI usage
3 malformed JSON, file I/O or output execution failure
```

The CLI never prints an address: outcomes carry refs, refusals name channels and refs, and a request that fails to decode is reported by file name only.

## 16. State and dependencies

No queue, database or persistence. File output is a delivery side effect only. Go 1.26, standard library only (`net/smtp`, `crypto/tls`, `net/http`, `net/mail`, `mime`).

## 17. Security boundary

Configuration is trusted deployment input. Requests are semi-trusted: the caller chooses addresses, so each per-recipient channel's anchored pattern is the relay boundary, and email addresses must also be bare mailboxes, which blocks header injection. Credentials live in the environment, never in configuration or requests. The component does not authorise recipients, compose content, store addresses or discover targets from messages.

## 18. Test/CI strategy

Package tests run against local servers only: a STARTTLS SMTP server with a generated certificate, an HTTPS gateway (`httptest.NewTLSServer`) and plain HTTP webhooks. CI adds Go 1.26 `gofmt`, vet, race tests, native build, exact version, Linux ARM64 cross-build, Bafflewick local delivery, and an offline per-recipient exercise run.

## 19. Known limitations

- file timeout is pre-I/O cancellation only;
- deliveries are sequential;
- no retry, queue or receipt;
- email supports AUTH PLAIN only, over STARTTLS only (no implicit TLS on port 465);
- the `text` body shape suits Slack and classic Teams incoming webhooks; Teams Workflows adaptive cards are not shaped;
- the `Date` header uses the host clock;
- no constrained-hardware benchmark; ARM64 is build proof only.

## 20. Acceptance-to-evidence map

| PRD criterion | Evidence |
|---|---|
| 1 local delivery | `TestLocalAndHTTPOutcomes`, Bafflewick CI step |
| 2 HTTP outcomes | `TestLocalAndHTTPOutcomes`, `TestPerRecipientSMSHTTP` |
| 3 timeouts bounded per delivery | `TestTimeoutVisible`, `TestTimeoutsBoundedPerDelivery` |
| 4 external by type | `TestHTTPCannotMasqueradeAsLocal`, `TestValidatePerRecipientConfig` |
| 5 no redirects | `TestHTTPRedirectIsNotFollowed` |
| 6 exercise suppression, every type | `TestExerciseSuppressesExternalBeforeIO`, `TestExerciseSuppressesEveryExternalTypeBeforeIO` |
| 7 failure isolation | `TestOneFailureDoesNotHideLaterOutcome`, `TestTimeoutsBoundedPerDelivery` |
| 8 duplicates refused | `TestValidateConfigAndRequest`, `TestValidateDeliveriesRefuseDoubleSendWithoutNamingAddress` |
| 9 blank/timeout refusals | `TestValidateConfigAndRequest` |
| 10 exit codes | `TestCLINeverPrintsAddresses`, `TestCLIVersionAndUsage` |
| 11 CI gate | `.github/workflows/test.yml` |
| 12 per-recipient email and sms-http | `TestPerRecipientEmailOverSTARTTLS`, `TestPerRecipientSMSHTTP`, `TestHTTPTextBodyShape` |
| 13 pattern refusal before I/O | `TestPatternRefusalBeforeIO`, `TestUnvalidatedPerRecipientChannelIsNeverOpen` |
| 14 address never output | `TestEmailRejectionNeverEchoesAddress`, `TestCLINeverPrintsAddresses`, `assertNoAddress` in every per-recipient test |
| 15 STARTTLS required | `TestEmailRequiresSTARTTLS`, `TestEmailUntrustedCertificateFails`, `TestEmailBadCredentialsAndMissingCredentials` |
| 16 mixed requests | `TestMixedFixedAndPerRecipient` |
| 17 v0.1 compatibility | `TestV01RequestAndConfigCompatibility`, unchanged v0.1 tests |
