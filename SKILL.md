# Notifier — Skill

## Use this when

Use this component when an application already has a message and an authorised set of channels and needs visible local/HTTP delivery attempts without owning vendor SDKs or a retry queue.

## Core rule

> **Delivery is an outcome to record, not an assumption to hide.**

## Change discipline

1. HTTP is always external; never make exercise suppression depend only on a caller flag.
2. Keep message composition and recipient authorisation upstream.
3. Return one outcome per requested channel in order.
4. Never let one channel failure hide later outcomes.
5. Refuse duplicate requested channels rather than risk accidental double-send.
6. Keep timeouts bounded and explicit.
7. Do not add retries unless the one-job boundary is deliberately changed.
8. Keep 2xx semantics as adapter acceptance, not end-recipient receipt.
9. Run format, vet, race, native, ARM64 and Bafflewick gates after changes.
10. Regenerate code-derived evidence before release.
