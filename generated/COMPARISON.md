# Notifier — Design / Implementation Comparison

**Implementation evidence:** `71ae48f15f8966e2599f6146e52e2766966cc52c`  
**Generated:** 2026-10-17

| Design area | Intended contract | Recovered implementation | Assessment |
|---|---|---|---|
| one-job boundary | attempt already-composed notifications and expose outcomes | one core package plus thin CLI; no composition/recipient/retry subsystem | MATCH |
| local delivery | work without network | file append adapter | MATCH |
| HTTP delivery | explicit webhook adapter | context-bound JSON POST; 2xx delivered, other responses failed | MATCH |
| exercise safety | suppress external I/O before send | HTTP classified external by type and suppressed before context/network creation | MATCH |
| external metadata | cannot bypass exercise control | HTTP must be marked external, but runtime classification does not rely on the flag | MATCH |
| timeout | bounded per-channel budget | 3000 ms default, maximum 60000 ms, child context per attempt | MATCH |
| duplicate request | avoid accidental double-send | duplicate requested channel names refused | MATCH |
| failure isolation | one failure must not hide later outcomes | iteration continues after unknown/failed channel | MATCH |
| retry | none | no retry/backoff/failover code | MATCH |
| CLI refusals | semantic refusal distinct from malformed execution input | exit 1 vs exit 3 implemented in CLI | MATCH |
| dependencies | Go standard library only | no third-party modules | MATCH |
| CI | format/vet/race/version/ARM64/acceptance | encoded in `test.yml` | MATCH |
| code-derived evidence | pin to source snapshot | generated EDD pins `71ae48f15f8966e2599f6146e52e2766966cc52c` | MATCH |

## Material corrections made before publication

The draft trusted a configurable `external` boolean strongly enough that an HTTP webhook could be marked non-external and contacted during an exercise. The published source makes HTTP external by construction: validation requires the metadata flag and runtime suppression derives externality from the channel type.

The draft CLI also collapsed semantic configuration/request errors into generic execution failures. The published CLI maps deterministic semantic refusals to exit 1 and reserves exit 3 for malformed JSON, file I/O and output failures.

Duplicate requested channels and unbounded supplied timeouts were also tightened before release.

## Result

**MATCH after correction.** The published implementation is aligned with the hand-written PRD/EDD and is stronger at the exercise-safety and failure-contract boundaries than the initial draft. No unexplained mismatch remains at the evidence commit.
