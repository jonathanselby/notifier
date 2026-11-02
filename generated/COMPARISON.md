# Notifier — Design / Implementation Comparison

**Implementation evidence:** `ad9ddf1c8dc1974dae2d7dc8e51dbc23c1701e7d`  
**Generated:** 2026-11-02  
**Targets:** hand-written `PRD.md` and `EDD.md` (v0.2); CRAB `docs/notifier-v0.2-requirement.md`

| Design area | Intended contract | Recovered implementation | Assessment |
|---|---|---|---|
| one-job boundary | attempt already-composed notifications and expose outcomes | one public package plus thin CLI; no composition, recipient, address-store or retry subsystem | MATCH |
| importability | consumable by applications | `notify` is public (moved from `internal/notify`) | MATCH |
| local delivery | work without network | file append adapter, unchanged | MATCH |
| HTTP delivery | explicit webhook; Slack/Teams-style body | `message` (default) or `text` body; 2xx delivered | MATCH |
| per-recipient addressing | `deliveries[{channel,to,ref}]`; fixed channels and v0.1 requests unchanged | channels then deliveries in one work list; addressing derived from type and checked per item | MATCH |
| allow-pattern | non-matching `to` refused as `address refused` before I/O | anchored pattern required at validation and re-checked in `Send`; refusals precede exercise and I/O | MATCH (stronger: anchoring required) |
| address containment | `ref` echoed, `to` never in outcomes, errors, logs | no address field; SMTP replies reported by code; case-insensitive scrub; CLI decode errors by file name | MATCH |
| email | SMTP, STARTTLS required, auth by env var name, fixed sender | `net/smtp`; STARTTLS mandatory before AUTH/MAIL; PLAIN from `auth_env`; `from` from config | MATCH |
| sms-http | HTTPS POST, body template with `{to}`, `{message}` | https enforced at validation and send; single-pass JSON-escaped template; optional `Authorization` from env | MATCH (addition: optional auth header) |
| external by type | email, sms-http, http always external; file never | `IsExternal` is `type != file`; metadata only checked for agreement | MATCH |
| exercise safety | suppression before I/O for every external type | suppressed before context creation for all non-file types | MATCH |
| timeout | per-delivery budget | child context per attempt; SMTP socket deadline and close-on-cancel | MATCH |
| duplicate request | avoid double-send | one destination key set across channels and deliveries | MATCH |
| failure isolation | one failure must not hide later outcomes | loop continues on every failure path | MATCH |
| retry | none | no retry/backoff/failover code | MATCH |
| CLI | exit 1 vs exit 3; no addresses printed | unchanged exit map; tested address-free output | MATCH |
| dependencies | standard library only, no vendor SDKs | no third-party modules | MATCH |
| CI | format/vet/race/version/ARM64/acceptance | version compared with `VERSION`; per-recipient offline exercise step; named suite | MATCH |
| code-derived evidence | pin to source snapshot | generated EDD pins `ad9ddf1c8dc1974dae2d7dc8e51dbc23c1701e7d` | MATCH |

## Material corrections made before publication

The self review of the first v0.2 commit found three defects, fixed in the evidence commit's parent:

1. Validation began refusing channel names with surrounding spaces, which v0.1 accepted. That broke the "v0.1 configuration unchanged" contract, so it was removed.
2. `Send` with an sms-http channel that skipped `Validate` would have posted to a plain `http://` gateway. `sendSMS` now refuses a non-https target before I/O.
3. Tests read the local servers' recorded state without their locks. The reads are now locked.

## Deliberate differences from the CRAB requirement

- The requirement's illustrative `body_shape: "teams"` is implemented as `"text"`, because Slack and classic Teams incoming webhooks both take `{"text": …}`. Teams Workflows adaptive cards are not shaped (EDD §19).
- `auth_env` is also accepted on sms-http (as the `Authorization` value), because organisation gateways need credentials and the requirement keeps credentials out of configuration.
- Address patterns must be anchored, a stricter reading of "the component cannot become an open relay".

## Result

**MATCH after correction.** The implementation meets the v0.2 PRD/EDD and the CRAB requirement, with the deliberate differences above. No unexplained mismatch remains at the evidence commit.
