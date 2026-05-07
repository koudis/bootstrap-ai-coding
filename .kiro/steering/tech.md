# Tech Stack

## Language & Runtime

- **Go** (primary language)
- Module path: `github.com/koudis/bootstrap-ai-coding`

## Key Libraries & Frameworks

- **[Cobra](https://github.com/spf13/cobra)** — CLI flag/command parsing
- **Docker SDK for Go** (`github.com/docker/docker/client`) — Docker daemon interaction
- **`golang.org/x/crypto/ssh`** — SSH host key generation (ed25519)
- **`encoding/json`** (stdlib) — agent manifest serialisation (`/bac-manifest.json`)
- **[`pgregory.net/rapid`](https://github.com/flyingmutant/rapid)** — property-based testing
- **[testify](https://github.com/stretchr/testify)** (`require` package) — test assertions

## External Dependencies

- Docker daemon >= `constants.MinDockerVersion` (20.10) must be running on the host
- Node.js LTS + npm (installed inside the container image by the Claude Code agent module)

## Common Commands

```bash
# Build
go build ./...

# Run tests (unit + property-based)
go test ./...

# Run integration tests (requires Docker daemon)
BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m -p 1 ./...

# Run a specific package's tests
go test ./naming/...
go test ./docker/...
go test ./constants/...
go test ./datadir/...

# Vet
go vet ./...

# Run the CLI
go run . <project-path>
go run . <project-path> --agents claude-code
go run . <project-path> --agents claude-code --port 2222
go run . <project-path> --ssh-key ~/.ssh/id_ed25519.pub
go run . <project-path> --rebuild
go run . <project-path> --stop-and-remove
go run . --purge
```

## Testing Conventions

- Unit and property-based tests: no build tag, run with `go test ./...`
- Integration tests: gated by `//go:build integration`, require a live Docker daemon, must use `-p 1` to avoid cross-package Docker state races
- Property tests use `rapid.Check(t, func(t *rapid.T) { ... })` with minimum 100 iterations
- Property test tag format: `// Feature: bootstrap-ai-coding, Property N: <property text>`
- Coverage target: ≥ 80% line coverage on all non-integration packages
