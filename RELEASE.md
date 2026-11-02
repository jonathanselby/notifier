# Release checklist

## Contract
- [ ] `VERSION`, CLI version and tag agree.
- [ ] Go 1.26 baseline is current.
- [ ] every non-file type is structurally external and exercise suppression occurs before I/O.
- [ ] per-recipient addresses are checked against an anchored pattern before I/O.
- [ ] no address appears in outcomes, refusals or CLI output.
- [ ] email sends nothing without STARTTLS to a trusted certificate.
- [ ] v0.1 configuration and requests still work unchanged.
- [ ] config/request validation tests cover duplicate and timeout boundaries.
- [ ] one failed channel cannot hide later outcomes.
- [ ] no retry/receipt/vendor-SDK claims have leaked in.

## Engineering gate
- [ ] `gofmt` clean.
- [ ] `go vet ./...` passes.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] native/version and Linux ARM64 builds pass.
- [ ] Bafflewick acceptance passes (local and per-recipient exercise).

## Evidence
- [ ] PRD acceptance criteria map to executable evidence.
- [ ] generated EDD pins the exact source snapshot.
- [ ] generated comparison explains any correction or mismatch.
