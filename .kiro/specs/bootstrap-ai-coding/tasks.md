# Tasks — Module Consolidation (Req 28)

Merge `internal/credentials` and `internal/portfinder` into `internal/datadir`.

## 1. Merge credentials into datadir

- [x] 1.1 Create `internal/datadir/credentials.go` with `ResolveCredentialPath` and `EnsureCredentialDir` functions (same logic as `credentials.Resolve` and `credentials.EnsureDir`)
- [x] 1.2 Create `internal/datadir/credentials_test.go` — move all tests from `internal/credentials/store_test.go`, updating import paths from `credentials` to `datadir`
- [x] 1.3 Update `internal/cmd/root.go` — replace `credentials.Resolve` with `datadir.ResolveCredentialPath` and `credentials.EnsureDir` with `datadir.EnsureCredentialDir`; remove the `credentials` import
- [x] 1.4 Delete `internal/credentials/store.go` and `internal/credentials/store_test.go`
- [x] 1.5 Verify build passes: `go build ./...`

## 2. Merge portfinder into datadir

- [x] 2.1 Create `internal/datadir/portfinder.go` with `FindFreePort` and `IsPortFree` functions (same logic as `portfinder.FindFreePort` and `portfinder.IsPortFree`)
- [x] 2.2 Create `internal/datadir/portfinder_test.go` — move all tests from `internal/portfinder/portfinder_test.go`, updating import paths from `portfinder` to `datadir`
- [x] 2.3 Update `internal/cmd/root.go` — replace `portfinder.FindFreePort` with `datadir.FindFreePort` and `portfinder.IsPortFree` with `datadir.IsPortFree`; remove the `portfinder` import
- [x] 2.4 Delete `internal/portfinder/portfinder.go` and `internal/portfinder/portfinder_test.go`
- [x] 2.5 Verify build passes: `go build ./...`

## 3. Run full test suite

- [x] 3.1 Run `go test ./...` and confirm all unit and property-based tests pass
- [x] 3.2 Run `go vet ./...` and confirm no issues
