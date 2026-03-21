# dep-health — Progress Tracker

> Last updated: 2026-03-21 (session 2)
> Use this file to orient a new agent or pick up after a context reset.

---

## What this project is

A Go CLI tool + web dashboard for scanning repositories for outdated dependencies, scoring them by risk, detecting peer conflict cascades, and generating AI-powered upgrade guidance. Built for a hackathon.

**Two entry points:**
- `dep-health scan [dir|--git-url]` — CLI, prints a ranked table
- `dep-health serve` — starts an HTTP server + embedded React dashboard

---

## Current state: working end-to-end

The full pipeline runs today:

```
git clone (optional) → scanner.Discover → resolver.Enrich → scorer.Score
→ scorer.DetectConflicts → advisor.Advise → table output / REST API
```

```bash
# Local scan
dep-health scan ./my-project

# Remote scan (clones with --depth 1, then deletes the clone)
dep-health scan --git-url https://github.com/org/repo

# Start the dashboard
dep-health serve --port 8080
# open http://localhost:8080
```

---

## Checklist

### Models (`models/models.go`)

- [x] `Dependency` struct (Name, CurrentVersion, Ecosystem, ManifestPath, RepoURL)
- [x] `EnrichedDependency` struct (embeds Dependency + LatestVersion, SeverityGap, VersionsBehind, Vulnerabilities, PeerConstraints)
- [x] `Vulnerability` struct (ID, Severity, Summary, URL)
- [x] `ScoredDependency` struct (embeds EnrichedDependency + RiskScore, CrossRepoCount, Reasons, BlockedBy, CascadeGroup)
- [x] `AdvisoryReport` struct (embeds ScoredDependency + Summary, BreakingChanges, MigrationSteps, PRUrl)
- [x] JSON tags on all structs

### Scanner (`scanner/`)

- [x] `Scanner` interface (`Name`, `Matches`, `Parse`)
- [x] `package.json` parser — `dependencies` + `devDependencies`, range operators stripped
- [x] `go.mod` parser — direct + indirect deps, local replacements excluded, `golang.org/x/mod/modfile`
- [x] `requirements.txt` parser — `RequirementsTxtScanner`, handles `==`, `>=`, `~=`, extras, env markers, comments
- [ ] `pom.xml` / `build.gradle` parser
- [x] Tests — `scanner_test.go`, `gomod_test.go`, `requirements_test.go` (parsing, version stripping, dir skipping, env markers, extras, options)

### Resolver (`resolver/resolver.go`)

- [x] npm registry lookup — `registry.npmjs.org/{pkg}`, `dist-tags.latest`, peer constraints
- [x] Go module proxy — `proxy.golang.org/{module}/@latest` + `@v/list`
- [x] OSV.dev batch API — single `POST /v1/querybatch` for all packages
- [x] Peer constraint extraction — `versions[latest].peerDependencies` → `PeerConstraints map[string]string`
- [x] Concurrent lookups — goroutines + semaphore (`chan struct{}`), configurable cap
- [x] Testable via injected URLs — `NPMRegistryURL`, `OSVBatchURL` fields on `Resolver`
- [x] PyPI JSON API lookup — `resolvePyPI()` hitting `pypi.org/pypi/{pkg}/json`, `PyPIRegistryURL` override field for tests
- [ ] Maven Central lookup
- [x] Tests — `resolver_test.go` with `httptest` mock server (peer constraints, versions-behind, 404, concurrent batch)

### Scorer (`scorer/`)

- [x] Weighted risk formula — CVE 40%, version gap 30%, versions-behind 20%, cross-repo 10%
- [x] Peer conflict detection — semver constraint checking via `Masterminds/semver/v3`
- [x] Cascade group assignment — union-find, lexicographic root for determinism
- [x] `BlockedBy` detection — set when peer's latest can't satisfy the constraint
- [x] Sorted output — descending by `RiskScore`
- [x] Tests — `conflict_test.go`, 9 cases (cascade, blocked, three-way, mixed, empty, determinism)

### Advisor (`advisor/advisor.go`)

- [x] `Advisor` interface — `Advise(ctx, ScoredDependency) (AdvisoryReport, error)`
- [x] `StubAdvisor` — deterministic summary + breaking-change warnings + ecosystem migration steps
- [x] `AnthropicAdvisor.Advise()` — full implementation in `advisor/anthropic.go`, activated via `ANTHROPIC_API_KEY`, falls back to stub on any error
- [ ] Changelog fetching — GitHub Releases API / CHANGELOG.md
- [ ] PR creation — `PRUrl` field exists in `AdvisoryReport`, nothing populates it yet
- [ ] Cascade-aware bundled PR generation
- [x] Tests for advisor package — `advisor/advisor_test.go`, 8 cases (summary content, major/minor breaking, ecosystem steps, CVE step, determinism, embedded dep)

### Pipeline (`pipeline/pipeline.go`)

- [x] `pipeline.Run()` — single entry point used by both CLI and server
- [x] `Options.GitURL` — clones repo to temp dir before scanning, injects `GITHUB_TOKEN` for HTTPS
- [x] `Options.OnProgress` — progress callback (CLI prints to stderr, server passes nil)
- [x] Temp dir cleanup via `defer os.RemoveAll`

### CLI (`cmd/`)

- [x] Cobra root command
- [x] `scan` subcommand — `--git-url`, `--repo`, `--top`, `--min-score`, `--json`
- [x] `serve` subcommand — `--port`, `--db`
- [x] Colour-coded table output (`tablewriter`)
- [x] Cascade group section below table
- [x] Blocked dependency section below table
- [x] Migration hints for top 3 packages
- [x] `--json` flag — emits `[]AdvisoryReport` as indented JSON
- [ ] `--create-prs` flag

### Store (`store/store.go`)

- [x] SQLite via `modernc.org/sqlite` (pure Go, no CGo)
- [x] `scan_runs` table — id, dir, repo_url, status, started_at, finished_at, error
- [x] `scan_deps` table — full `AdvisoryReport` fields, JSON columns for arrays/maps
- [x] `CreateScanRun`, `FinishScanRun`, `SaveDeps`, `ListScans`, `GetScan`
- [x] `RecoverStuckScans` — marks interrupted runs as failed on startup
- [x] WAL mode + foreign keys enabled

### Server (`server/server.go`)

- [x] Go 1.22 `ServeMux` with `{id}` path parameters
- [x] `GET /api/scans` — list all runs
- [x] `GET /api/scans/{id}` — run + full dep report
- [x] `POST /api/scans` — async trigger (202 Accepted), accepts `dir` or `git_url`
- [x] SPA fallback — serves `dist/index.html` for unmatched routes
- [x] In-flight scan tracking with `context.CancelFunc` map

### Frontend (`frontend/src/`)

- [x] Vite + React 18 + React Router v6
- [x] Embedded into binary via `//go:embed dist` (`web/embed.go`)
- [x] `ScanList` page — trigger form (local/remote toggle), history table, auto-polls while running
- [x] `ScanDetail` page — run metadata, deps table, migration hints, cascade + blocked panels
- [x] `DepsTable` — colored left-border stripe per cascade group, colored CASCADE badge
- [x] `CascadePanel` — matching group colors, dot indicators
- [x] `BlockedPanel` — blocked deps with peer constraint details
- [x] `RiskBadge`, `StatusBadge` components
- [x] Dark theme CSS design system
- [x] `cascadeColor.js` — deterministic group→color mapping (hash → palette)

---

## Architecture summary

```
main.go
└── cmd/
    ├── scan     → pipeline.Run() → prints table
    └── serve    → server.New(store) → http.ListenAndServe

pipeline.Run()
  0. git clone --depth 1 (if GitURL set)
  1. scanner.Discover()     → []Dependency
  2. resolver.Enrich()      → []EnrichedDependency   (concurrent + OSV batch)
  3. scorer.Score()         → []ScoredDependency      (sorted desc)
  4. scorer.DetectConflicts → []ScoredDependency      (BlockedBy + CascadeGroup)
  5. advisor.Advise()       → []AdvisoryReport

Packages:   models ← scanner, resolver, scorer, advisor
                  ↑
            pipeline ← cmd, server
                            ↑
                         store, web
```

### Key files

| File | Purpose |
|---|---|
| `models/models.go` | All shared structs with JSON tags |
| `scanner/scanner.go` | `Scanner` interface + `PackageJSONScanner` + `Discover()` |
| `scanner/gomod.go` | `GoModScanner` using `golang.org/x/mod/modfile` |
| `resolver/resolver.go` | npm + Go proxy + OSV.dev, concurrent with semaphore |
| `scorer/scorer.go` | Weighted risk formula |
| `scorer/conflicts.go` | `DetectConflicts` + union-find |
| `advisor/advisor.go` | `Advisor` interface + `StubAdvisor` |
| `advisor/anthropic.go` | `AnthropicAdvisor` — full Anthropic API implementation via tool use |
| `pipeline/pipeline.go` | Orchestrates all stages, handles git clone |
| `store/store.go` | SQLite persistence |
| `server/server.go` | REST API + SPA handler |
| `cmd/scan.go` | CLI scan subcommand |
| `cmd/serve.go` | CLI serve subcommand |
| `frontend/src/` | React dashboard |
| `web/embed.go` | `//go:embed dist` |

### Dependencies (go.mod)

```
github.com/anthropics/anthropic-sdk-go v1.27.1
github.com/Masterminds/semver/v3 v3.2.1
github.com/olekukonko/tablewriter v0.0.5
github.com/spf13/cobra v1.8.1
golang.org/x/mod v0.22.0
modernc.org/sqlite v1.34.4
```

---

## What to build next (prioritised)

### 1. PR creation — closes the original pitch loop

Populate `AdvisoryReport.PRUrl`. Even a draft GitHub PR with the migration steps in the body closes the original pitch loop.
- `--create-prs` flag on `scan` subcommand
- GitHub API: `POST /repos/{owner}/{repo}/pulls`
- `GITHUB_TOKEN` already loaded in config

### 2. `pyproject.toml` / `setup.py` scanner — extends Python coverage

- Parse `[project] dependencies` (PEP 621) and `install_requires` (setuptools)
- Ecosystem: `"pypi"`, resolver already handles it

### 3. `pom.xml` / Maven — lower priority

Harder to demo, adds complexity. Skip unless Java-specific judges are expected.

---

## Environment variables

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | Activates `AnthropicAdvisor` (otherwise stub is used) |
| `GITHUB_TOKEN` | Injected into HTTPS git clone URLs for private repos; future PR creation |
| `DEP_HEALTH_ORG` | Organisation name for future multi-repo scanning |
| `DEP_HEALTH_MAX_CONCURRENCY` | Max parallel registry requests (default: 10) |
| `DEP_HEALTH_DB` | SQLite database path for server (default: `dep-health.db`) |

---

## Build instructions

```bash
# Go dependencies
go mod tidy

# Build and run frontend (required before go build)
cd frontend && npm install && npm run build && cd ..

# Compile binary (embeds frontend)
go build -o dep-health .

# Run tests
go test ./...
```
