# Release checklist

## Contract
- [ ] `VERSION`, CLI version and tag agree.
- [ ] Go 1.26 baseline is current.
- [ ] HTTP is structurally external and exercise suppression occurs before I/O.
- [ ] config/request validation tests cover duplicate and timeout boundaries.
- [ ] one failed channel cannot hide later outcomes.
- [ ] no retry/receipt/vendor claims have leaked into V0.1.

## Engineering gate
- [ ] `gofmt` clean.
- [ ] `go vet ./...` passes.
- [ ] `go test -race -count=1 ./...` passes.
- [ ] native/version and Linux ARM64 builds pass.
- [ ] Bafflewick acceptance passes.

## Evidence
- [ ] PRD acceptance criteria map to executable evidence.
- [ ] generated EDD pins the exact source snapshot.
- [ ] generated comparison explains any correction or mismatch.
