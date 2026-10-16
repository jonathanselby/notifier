# Notifier — Generated Engineering Design Document

**Evidence commit:** `71ae48f15f8966e2599f6146e52e2766966cc52c`  
**Method:** code-derived review using the `codebase-to-edd` evidence discipline  
**Generated:** 2026-10-17

This document describes the implementation at the evidence commit. The hand-written PRD/EDD are comparison targets, not runtime evidence.

## 1. Executive description

**KNOWN** — `notifier` is a small synchronous Go component with one material package, `internal/notify`, and a thin JSON-file CLI.

**KNOWN** — it attempts an already-composed message through explicitly requested configured channels and emits one ordered `delivered`, `failed` or `suppressed` outcome per request entry.

## 2. Evidence inspected

- `internal/notify/notify.go` — validation, classification, timeout and adapters;
- `internal/notify/notify_test.go` — behavioural evidence;
- `cmd/notifier/main.go` — process and exit contract;
- `.github/workflows/test.yml` — engineering gate;
- `examples/bafflewick/` — offline acceptance path;
- `go.mod`, `VERSION`, `CHANGELOG.md` — language/release boundary.

## 3. Runtime structure

```text
config + request
      |
      v
Validate + ValidateRequest
      |
      v
requested channels in order
      |
      +-- unknown -> failed
      +-- exercise + HTTP -> suppressed before I/O
      +-- file -> append
      +-- HTTP -> context-bound POST
      |
      v
Result{Outcomes, AllSucceeded}
```

## 4. Configuration contract

**KNOWN** — at least one configured channel is required; names and targets must be non-blank; names must be unique.

**KNOWN** — supported channel types are exactly `file` and `http`.

**KNOWN** — file channels are required to have `external:false`; HTTP channels are required to have `external:true`.

**KNOWN** — exercise classification does not trust the metadata flag alone: `isExternal` derives externality from `Type == "http"`.

## 5. Request contract

**KNOWN** — message must be non-blank and at least one channel must be requested.

**KNOWN** — requested channel names must be non-blank and unique. Duplicate channel names are refused before delivery, preventing an accidental double-send through one request.

**KNOWN** — timeout zero selects 3000 ms; supplied timeout must not exceed 60000 ms and may not be negative.

## 6. Exercise safety

**KNOWN** — exercise suppression is evaluated before a child context or network request is created.

**KNOWN** — every HTTP channel is suppressed in exercise mode while file channels remain eligible for delivery.

**KNOWN** — deliberate suppression does not set `AllSucceeded=false`.

## 7. File adapter

**KNOWN** — the file sink opens the target with create/append/write and mode `0600`, then appends the message plus newline.

**KNOWN** — context cancellation is checked before opening the file.

**KNOWN** — the code does not claim cancellation of an already-blocked filesystem syscall.

## 8. HTTP adapter

**KNOWN** — HTTP sends a POST with JSON `{"message": ...}` and `Content-Type: application/json`.

**KNOWN** — the request is bound to the per-channel context.

**KNOWN** — any 2xx is recorded as delivered; non-2xx is failed. Up to 64 KiB of response body is drained/discarded before close.

**KNOWN** — no webhook authentication or credential model exists in V0.1.

## 9. Timeout model

**KNOWN** — timeout is per attempted channel, not a whole-request deadline. A new child context is created for each attempted channel.

**INFERRED** — because attempts are sequential, worst-case wall time can approach requested-channel count multiplied by the per-channel timeout, subject to the parent context.

## 10. Failure isolation

**KNOWN** — an unknown channel, adapter error, HTTP non-2xx or timeout produces a failed outcome and processing continues to later requested channels.

**KNOWN** — there is no automatic retry, backoff or failover.

## 11. CLI contract

**KNOWN** — `-version` prints `notifier 0.1.0`.

**KNOWN** — exit mapping is:

```text
0 all attempted channels delivered or were deliberately suppressed
1 semantic config/request refusal or one/more delivery failures
2 usage error
3 malformed JSON, file I/O or output encoding failure
```

## 12. State and dependencies

**KNOWN** — there is no queue, database, durable delivery state or background worker.

**KNOWN** — file output is a side effect, not authoritative delivery persistence.

**KNOWN** — `go.mod` targets Go 1.26 and declares no third-party modules.

## 13. Security boundary

**KNOWN** — configuration is trusted deployment input.

**KNOWN** — the component does not compose messages, authorise recipients, discover webhook targets from message content or own credentials.

**KNOWN** — HTTP cannot be made exercise-safe merely by setting `external:false`; validation refuses that configuration and runtime classification is still type-derived.

## 14. Executable evidence

Tests include:

- `TestLocalAndHTTPOutcomes`;
- `TestExerciseSuppressesExternalBeforeIO`;
- `TestHTTPCannotMasqueradeAsLocal`;
- `TestTimeoutVisible`;
- `TestOneFailureDoesNotHideLaterOutcome`;
- `TestValidateConfigAndRequest`.

CI adds Go 1.26 formatting with diagnostic diff, vet, race tests, native build, exact version, Linux ARM64 cross-build, Bafflewick acceptance and the named delivery-boundary suite.

## 15. Bafflewick path

**KNOWN** — the default Bafflewick request uses only the local file sink, writes `Bafflewick break-glass activated`, and requires no network access.

## 16. Known limits / unknowns

**KNOWN** — no guaranteed delivery, delivery receipt, retry queue, recipient directory, template engine or vendor SDK.

**KNOWN** — filesystem cancellation is only checked before I/O; V0.1 cannot guarantee interruption of a wedged filesystem syscall.

**UNKNOWN** — runtime performance on the Garage constrained-hardware target. ARM64 CI is compilation proof only.

## 17. Evidence conclusion

**KNOWN** — the source implements the one-job notifier boundary and makes delivery failures/suppression explicit rather than implicit. The important safety boundaries are executable in tests rather than document-only claims.
