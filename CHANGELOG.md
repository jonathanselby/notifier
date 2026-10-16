# Changelog

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
