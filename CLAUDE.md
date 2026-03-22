# CLAUDE.md — dep-health

## Build & run

```bash
# One-command build + serve
./serve.sh

# Or step by step:
go mod tidy
cd frontend && npm install && npm run build && cd ..
go build -o dep-health .
```

## Test

```bash
go test ./...
```

No network calls in tests — all registry lookups use `httptest.Server` mocks.

## Project layout

- `models/` — shared structs (Dependency → Enriched → Scored → Advisory)
- `scanner/` — manifest parsers (npm, Go, Python, Java)
- `resolver/` — registry lookups + OSV.dev CVE batch
- `scorer/` — risk formula + peer conflict detection (union-find)
- `advisor/` — upgrade guidance (stub + Anthropic API)
- `pipeline/` — orchestrates all stages; shared by CLI and server
- `store/` — SQLite persistence (pure Go, no CGo)
- `server/` — HTTP REST API + SPA fallback + session middleware
- `web/` — `//go:embed dist` wrapper for compiled frontend
- `frontend/` — Vite + React 18 dashboard source
- `cmd/` — Cobra CLI (scan, scan-multi, serve)
- `testdata/` — fixture manifests with intentionally old versions

## Key conventions

- Domain packages (`scanner`, `resolver`, `scorer`, `advisor`) import only `models`
- `pipeline` orchestrates domain packages but never imports `server` or `store`
- Only `cmd/` and `server/` may import multiple domain packages
- Store uses additive `ALTER TABLE ADD COLUMN` migrations — duplicate-column errors are suppressed
- Frontend must be built before `go build` (assets are embedded via `//go:embed`)
- `StubAdvisor` is used when `ANTHROPIC_API_KEY` is unset — no API key needed for dev/CI
- Session isolation via anonymous cookie (`dep_health_session`) — all `/api/` routes are wrapped with session middleware
- All scan data is scoped to the session that created it

## Environment variables

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | Activates AI advisor (otherwise deterministic stub) |
| `GITHUB_TOKEN` | Injected into HTTPS clone URLs for private repos |
| `DEP_HEALTH_MAX_CONCURRENCY` | Max parallel registry requests (default: 10) |
| `DEP_HEALTH_DB` | SQLite database path for `serve` (default: `dep-health.db`) |

## Quick demo

```bash
./dep-health scan testdata/java-maven        # log4j CVEs
./dep-health scan testdata/cascade            # React peer-conflict cascade
./dep-health scan testdata/python-pyproject   # Django 55 CVEs
./dep-health serve                            # dashboard at :8080
```
