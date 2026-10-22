# Notifier — PRD

**Version:** 0.1

## Problem

An incident application may have local notification available while external channels are slow, absent or unsafe to contact during an exercise. A single channel failure must not erase later outcomes, and “sent” must never be inferred from an attempted call.

## Product objective

Provide a small deterministic Go component that attempts an already-composed notification through explicit configured channels and records the outcome of each attempt.

> **Delivery is an outcome to record, not an assumption to hide.**

## Users

Applications that need local and optional external alert delivery without embedding vendor or retry logic in the domain model.

## In scope

- local append-to-file sink;
- HTTP POST webhook adapter;
- HTTP delivery only to the explicitly configured target; redirects are not followed;
- per-channel timeout budget;
- `delivered`, `failed`, `suppressed` outcomes;
- exercise suppression before external I/O;
- deterministic config/request validation;
- independent attempts in caller-supplied order;
- no automatic retry;
- stable process exit codes;
- Go 1.26, standard library only.

## Out of scope

- message composition/templates;
- recipient directory/authorisation;
- email/SMS/push vendor SDKs;
- delivery-receipt protocols;
- guaranteed delivery;
- queue persistence/retry/backoff;
- escalation policy;
- webhook credential custody.

## Acceptance criteria

1. local delivery works without network access;
2. HTTP 2xx is recorded as delivered and non-2xx/network errors as failed;
3. a timed-out HTTP attempt is visibly failed;
4. every HTTP channel is treated as external and cannot bypass exercise suppression through configuration;
5. HTTP redirects are not followed and are recorded as failed;
6. exercise suppression occurs before external I/O while local delivery continues;
7. one failed/unknown channel does not hide later outcomes;
8. duplicate requested channels are refused rather than sent twice accidentally;
9. blank message/channel lists and invalid timeout bounds are refused;
10. semantic config/request refusals map to exit 1; malformed JSON/file I/O map to exit 3;
11. CI proves format, vet, race tests, version contract and Linux ARM64 build.

## Definition of done

V0.1 is done when all criteria have executable evidence, the operational/security boundary is explicit, code-derived EDD evidence is pinned to the implementation snapshot, and the design comparison contains no unexplained mismatch.
