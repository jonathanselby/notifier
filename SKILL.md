# Notifier — Skill

## Use this when

Use this component when an application already has a message, has decided who should receive it, and needs visible delivery attempts (local file, webhook, email or SMS gateway) without owning vendor SDKs, an address store or a retry queue.

## Core rule

> **Delivery is an outcome to record, not an assumption to hide.**

## Change discipline

1. External is derived from type; never make exercise suppression depend on a caller flag.
2. Keep message composition, recipient choice and address storage upstream.
3. Return one outcome per requested delivery, in order (channels first, then deliveries).
4. Never let one delivery failure hide later outcomes.
5. Refuse duplicate destinations rather than risk accidental double-send.
6. Keep timeouts bounded and explicit, per delivery.
7. Check every address against the channel's anchored pattern before any I/O.
8. Never put an address in an outcome, refusal or log; report server refusals by code, not text.
9. Email: STARTTLS or nothing; credentials only from the environment.
10. Do not add retries unless the one-job boundary is deliberately changed.
11. Keep 2xx semantics as adapter acceptance, not end-recipient receipt.
12. Keep v0.1 configuration and requests working.
13. Run format, vet, race, native, ARM64, Bafflewick and per-recipient gates after changes.
14. Regenerate code-derived evidence before release.
