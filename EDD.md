# Notifier — Engineering Design Document

**Version:** 0.1

## 1. Purpose

Attempt already-composed notifications through explicit local/HTTP channels and return one visible outcome for every requested channel.

> **Delivery is an outcome to record, not an assumption to hide.**

## 2. Design constraints

1. Local operation must not depend on external channels.
2. Exercise mode must suppress external I/O before the adapter runs.
3. A failed channel must not stop later channels.
4. No automatic retry or hidden failover.
5. HTTP configuration cannot masquerade as local.
6. Message/recipient policy remains upstream.

## 3. Runtime model

```text
config + request
      |
      v
validate channel types / external classification
validate message / channels / timeout / duplicates
      |
      v
for requested channel in order
      |
      +-- unknown ----------------------> failed outcome
      +-- exercise + HTTP -------------> suppressed (no I/O)
      +-- file -------------------------> append -> delivered/failed
      +-- HTTP -------------------------> POST -> delivered/failed
      |
      v
Result{Outcomes, AllSucceeded}
```

## 4. Core types

`Channel` names a `file` or `http` target. `Request` contains a composed message, ordered unique channel names, exercise flag and optional timeout. `Outcome` records channel, status, detail and elapsed milliseconds.

## 5. Configuration validation

`Validate` requires at least one channel, non-empty unique names, non-empty targets and supported types.

A file channel must have `external:false`; an HTTP channel must have `external:true`. Runtime exercise classification is nevertheless derived from `Type == "http"`, so the metadata field cannot be used to bypass suppression.

## 6. Request validation

`ValidateRequest` requires a non-blank message, at least one requested channel, non-empty unique requested channel names, and a timeout of zero (default) or 1–60000 ms.

Duplicate requested channel names are refused before delivery so one request cannot accidentally double-send the same channel.

## 7. Exercise suppression

For an exercise request, every HTTP channel is returned as `suppressed` before a context, request or network operation is created. File channels remain eligible for delivery.

Suppression is deliberate and does not set `AllSucceeded=false`.

## 8. File adapter

File delivery opens the configured path with create/append/write and mode `0600` for newly created files, then writes the message plus newline.

The context is checked before opening. V0.1 does not claim it can interrupt an already-blocking filesystem syscall.

## 9. HTTP adapter

HTTP delivery JSON-encodes `{"message": <text>}`, creates a context-bound POST, sets `Content-Type: application/json`, and uses either an injected client or a default `http.Client`.

Any 2xx response is `delivered`; non-2xx is failed. Up to 64 KiB of response body is drained/discarded before close. A 2xx means the adapter accepted the request, not that an end recipient received it.

## 10. Timeout model

Timeout is per attempted channel, not a whole-request deadline. Zero selects 3000 ms. The caller's parent context is inherited by each per-channel child context.

## 11. Failure isolation

Unknown channels, adapter errors, non-2xx and timeouts produce a failed outcome and set `AllSucceeded=false`; iteration continues to later requested channels.

No retry/backoff is performed.

## 12. CLI contract

```text
notifier -config <channels.json> -request <notification.json>
notifier -version
```

Exit codes:

```text
0 all attempted channels delivered or were suppressed
1 semantic config/request refusal or one/more delivery failures
2 invalid CLI usage
3 malformed JSON, file I/O or output execution failure
```

## 13. State and dependencies

No queue, database or authoritative persistence exists. File output is a delivery side effect only. The module targets Go 1.26 and uses the standard library only.

## 14. Security boundary

Configuration is trusted deployment input. The component does not authorise recipients, compose sensitive content, discover arbitrary webhook targets from messages, or own webhook credentials.

Exercise safety is enforced from channel type, not solely from caller metadata.

## 15. Test/CI strategy

Package tests prove:

- local delivery and visible HTTP failure;
- exercise HTTP suppression before I/O while local delivery continues;
- HTTP cannot be configured as non-external;
- timeout failure is visible;
- one failure does not hide a later successful channel;
- invalid config/request forms are refused.

CI adds Go 1.26 `gofmt`, vet, race tests, native build, exact version, Linux ARM64 cross-build and Bafflewick local-delivery acceptance.

## 16. Known limitations

- file timeout is pre-I/O cancellation, not guaranteed interruption of blocked filesystem calls;
- no HTTP authentication fields;
- no retry/backoff/durable queue;
- no delivery receipt protocol;
- per-channel sequential attempts mean total request time can approach channel-count × timeout;
- no constrained-hardware runtime benchmark in V0.1; ARM64 is build proof only.

## 17. Acceptance-to-evidence map

| Requirement | Evidence |
|---|---|
| local + visible HTTP failure | `TestLocalAndHTTPOutcomes` |
| exercise suppression before external I/O + local continues | `TestExerciseSuppressesExternalBeforeIO` |
| HTTP cannot masquerade as local | `TestHTTPCannotMasqueradeAsLocal` |
| timeout visible | `TestTimeoutVisible` |
| failure isolation | `TestOneFailureDoesNotHideLaterOutcome` |
| config/request validation | `TestValidateConfigAndRequest` |
| format/vet/race/build/version/ARM64/Bafflewick | `.github/workflows/test.yml` |
