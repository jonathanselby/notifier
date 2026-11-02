# Notifier — PRD

**Version:** 0.2

## Problem

An incident application may have local notification available while external channels are slow, absent or unsafe to contact during an exercise. A single channel failure must not erase later outcomes, and "sent" must never be inferred from an attempted call.

In v0.1 a channel was a fixed target, so the only way to reach a specific person was a per-person channel in deployment configuration. That put personal data in deployment config and needed a restart for every new person. v0.2 lets a request address a person on a shared transport (an SMTP relay or an SMS gateway) without the address leaking into outcomes or logs.

## Product objective

Provide a small deterministic Go component that attempts an already-composed notification through explicit configured channels, to fixed targets or to per-delivery addresses the channel allows, and records the outcome of each attempt.

> **Delivery is an outcome to record, not an assumption to hide.**

## Users

Applications that need local and optional external alert delivery without embedding vendor or retry logic in the domain model. The first consumer is CRAB (`docs/notifier-v0.2-requirement.md` in that repository).

## In scope

- local append-to-file sink;
- HTTP POST webhook, with a `message` (v0.1) or `text` (Slack/Teams incoming webhook) body;
- **email** by SMTP submission, STARTTLS required, credentials named by environment variable, fixed sender;
- **sms-http**: HTTPS POST to an organisation gateway with a JSON body template using `{to}` and `{message}`;
- per-recipient addressing for email and sms-http, gated by an anchored allow-pattern per channel;
- outcomes that echo the caller's `ref` and never the address;
- HTTP delivery only to the explicitly configured target; redirects are not followed;
- per-delivery timeout budget;
- `delivered`, `failed`, `suppressed` outcomes;
- exercise suppression before external I/O for every external type;
- deterministic config/request validation;
- independent attempts in caller-supplied order;
- no automatic retry;
- stable process exit codes;
- a public Go package (`notify`) and a thin CLI;
- Go 1.26, standard library only.

## Out of scope

- composing messages or templates beyond the sms-http body template;
- choosing recipients, recipient directory or authorisation;
- storing addresses;
- debounce;
- vendor SDKs;
- delivery-receipt protocols or read receipts;
- guaranteed delivery;
- queue persistence, retry or backoff;
- escalation chains;
- two-way messaging.

## Acceptance criteria

1. local delivery works without network access;
2. HTTP 2xx is recorded as delivered and non-2xx/network errors as failed;
3. a timed-out attempt is visibly failed, for every channel type, and bounded per delivery;
4. every non-file channel is external by type and cannot bypass exercise suppression through configuration;
5. HTTP redirects are not followed and are recorded as failed;
6. exercise suppression occurs before external I/O for http, email and sms-http while local delivery continues;
7. one failed/unknown channel does not hide later outcomes;
8. duplicate requested channels, and duplicate deliveries to the same destination, are refused rather than sent twice;
9. blank message/channel lists and invalid timeout bounds are refused;
10. semantic config/request refusals map to exit 1; malformed JSON/file I/O map to exit 3;
11. CI proves format, vet, race tests, version contract and Linux ARM64 build;
12. per-recipient delivery works for email (against a local STARTTLS SMTP server) and sms-http (against a local HTTPS gateway);
13. an address that fails the channel's allow-pattern is refused as `failed`, detail `address refused`, before any I/O; a per-recipient channel without a valid anchored pattern never sends;
14. the address is absent from outcomes, refusals and CLI output, including when a server's refusal text echoes it;
15. email sends nothing, and offers no credentials, unless STARTTLS succeeds with a trusted certificate;
16. requests mixing fixed channels and per-recipient deliveries work, in order;
17. v0.1 configuration and requests are accepted unchanged and produce the v0.1 outcome shape.

## Definition of done

V0.2 is done when all criteria have executable evidence, the operational/security boundary is explicit, code-derived EDD evidence is pinned to the implementation snapshot, and the design comparison contains no unexplained mismatch.
