# dep-health — Progress Tracker

> Last updated: 2026-03-21 (session 4)
> Use this file to orient a new agent or pick up after a context reset.

---

## What this project is

A Go CLI tool + web dashboard for scanning repositories for outdated dependencies, scoring them by risk, detecting peer conflict cascades, and generating AI-powered upgrade guidance. Built for a hackathon.

**Three entry points:**
- `dep-health scan [dir|--git-url]` — CLI, prints a ranked table + cascade/blocked/migration sections
- `dep-health scan-multi <target> <target> [...]` — scans multiple repos and aggregates with cross-repo scoring
- `dep-health serve` — starts an HTTP server + embedded React dashboard with async scan history

**Scope constraints established:**
- PR creation is **explicitly out of scope** — no GitHub integration; the org uses GitLab with internal repos
- There is no multi-repo / org-wide scanning target for this iteration

---

## Current state: working end-to-end

The full pipeline runs today for all four ecosystems:

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

All tests pass: `go test ./...`

---

## Checklist

### Models (`models/models.go`)

- [x] `Dependency` struct (Name, CurrentVersion, Ecosystem, ManifestPath, RepoURL)
- [x] `EnrichedDependency` struct (embeds Dependency + LatestVersion, SeverityGap, VersionsBehind, Vulnerabilities, PeerConstraints)
- [x] `Vulnerability` struct (ID, Severity, Summary, URL)
- [x] `ScoredDependency` struct (embeds EnrichedDependency + RiskScore, CrossRepoCount, **RepoSource**, Reasons, BlockedBy, CascadeGroup)
- [x] `AdvisoryReport` struct (embeds ScoredDependency + Summary, BreakingChanges, MigrationSteps, PRUrl)
- [x] `MultiRepoReport` struct — Targets, PerRepo, AllDeps, Stats, Errors
- [x] `MultiRepoStats` struct — TotalRepos, TotalOutdated, TotalCVEs, CascadeGroups, BlockedDeps
- [x] JSON tags on all structs

### Scanner (`scanner/`)

- [x] `Scanner` interface (`Name`, `Matches`, `Parse`)
- [x] `PackageJSONScanner` — `package.json`, dependencies + devDependencies, range operators stripped
- [x] `GoModScanner` — `go.mod`, direct + indirect deps, local replacements excluded, uses `golang.org/x/mod/modfile`
- [x] `RequirementsTxtScanner` — `requirements.txt` + `requirements-*.txt`, handles `==`, `>=`, `~=`, `<=`, extras, env markers, pip options (`-r`, `--index-url`)
- [x] `PyprojectScanner` — `pyproject.toml`, PEP 621 `[project] dependencies` + optional-dependencies, Poetry `[tool.poetry.dependencies]` + dev-dependencies + groups, uses `github.com/BurntSushi/toml`
- [x] `SetupCfgScanner` — `setup.cfg`, `install_requires` + `extras_require`, multi-line continuation, inline comments
- [x] `PomScanner` — `pom.xml`, `<dependencies>`, `<dependencyManagement>`, `<parent>`, `${property}` resolution, system-scope skipped, Java 8 compatible
- [x] `DefaultScanners()` registers all six
- [x] Tests — `scanner_test.go`, `gomod_test.go`, `requirements_test.go`, `pyproject_test.go`, `setupcfg_test.go`, `pom_test.go`

### Resolver (`resolver/resolver.go`)

- [x] npm registry lookup — `registry.npmjs.org/{pkg}`, `dist-tags.latest`, peer constraints
- [x] Go module proxy — `proxy.golang.org/{module}/@latest` + `@v/list`
- [x] PyPI JSON API — `pypi.org/pypi/{pkg}/json`, pre-releases excluded from versions-behind count
- [x] Maven Central search API — `search.maven.org/solrsearch/select`, `g:{g}+AND+a:{a}`, `core=gav` for version list
- [x] OSV.dev batch API — single `POST /v1/querybatch` for all packages; ecosystem mapping: npm→npm, pypi→PyPI, go→Go, maven→Maven
- [x] Peer constraint extraction — `versions[latest].peerDependencies` → `PeerConstraints map[string]string`
- [x] Concurrent lookups — goroutines + semaphore (`chan struct{}`), configurable via `DEP_HEALTH_MAX_CONCURRENCY` (default 10)
- [x] Testable via injected URLs — `NPMRegistryURL`, `OSVBatchURL`, `PyPIRegistryURL`, `MavenCentralURL` fields on `Resolver`
- [x] Tests — `resolver_test.go` with `httptest` mock server (peer constraints, versions-behind, 404, concurrent batch)

### Scorer (`scorer/`)

- [x] Weighted risk formula — CVE severity 40%, version gap 30%, versions-behind 20%, cross-repo count 10%
- [x] Peer conflict detection — semver constraint checking via `Masterminds/semver/v3`
- [x] Cascade group assignment — union-find, lexicographic root for determinism
- [x] `BlockedBy` detection — set when peer's latest can't satisfy the constraint
- [x] Sorted output — descending by `RiskScore`
- [x] Tests — `conflict_test.go`, 9 cases (cascade, blocked, three-way, mixed, empty, determinism)

### Advisor (`advisor/`)

- [x] `Advisor` interface — `Advise(ctx, ScoredDependency) (AdvisoryReport, error)`
- [x] `StubAdvisor` — deterministic summary + breaking-change warnings + ecosystem migration steps (npm/pypi/go/maven)
- [x] `AnthropicAdvisor` — `advisor/anthropic.go`, forced `record_advisory` tool call for structured JSON output, activated via `ANTHROPIC_API_KEY`, falls back to stub on any error
- [x] Tests — `advisor_test.go`, 8 cases (summary content, major/minor breaking changes, NPM/PyPI/Go ecosystem steps, CVE step, determinism, embedded dep)
- [ ] Changelog fetching — GitHub Releases API / CHANGELOG.md scraping (not started)

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
- [x] `looksLikeGitURL()` auto-promotion — if `dir` looks like a remote URL (`https://`, `http://`, `git@`, `git://`) and `git_url` is unset, silently promotes to git clone path

### Frontend (`frontend/src/`)

- [x] Vite + React 18 + React Router v6
- [x] Embedded into binary via `//go:embed dist` (`web/embed.go`)
- [x] `ScanList` page — trigger form (local/remote toggle), scan history table, auto-polls while running
- [x] `ScanDetail` page — run metadata, deps table, migration hints, cascade + blocked panels
- [x] `DepsTable` — colored left-border stripe per cascade group, CASCADE/BLOCKED badges
- [x] `CascadePanel` — matching group colors, dot indicators
- [x] `BlockedPanel` — blocked deps with peer constraint details
- [x] `MigrationPanel` — top-3 riskiest packages with steps and breaking-change warnings
- [x] `RiskBadge`, `StatusBadge` components
- [x] Dark theme CSS design system
- [x] Deterministic cascade group → color mapping (hash → 6-color palette)
- [x] Auto-detect URL pasted into local path field — switches to "Git URL" mode automatically

### Test fixtures (`testdata/`)

| Fixture | Ecosystems | Highlights |
|---|---|---|
| `testdata/npm-only` | npm | lodash@3, axios@0.21 — CVEs, babel+webpack cascade group |
| `testdata/python-only` | requirements.txt | Flask@1.1, Werkzeug@1 — 13 CVEs on Werkzeug |
| `testdata/python-pyproject` | pyproject.toml (PEP 621) | Django@3.2 — 55 known CVEs |
| `testdata/python-setupcfg` | setup.cfg | Flask + SQLAlchemy, extras_require sections |
| `testdata/java-maven` | pom.xml | Spring Boot 2.6 / Java 8, log4j 2.14.1 pre-Log4Shell (5 CVEs) |
| `testdata/go-only` | go.mod | gin@1.7.7, cobra@1.2.1, viper@1.9.0 |
| `testdata/mixed` | npm + Python + Go | Multi-ecosystem in a single directory |
| `testdata/cascade` | npm | React@16 — 4-package cascade (react, react-dom, react-router-dom, @testing-library/react) |
| `testdata/no-deps` | — | Empty manifests — graceful-empty handling |

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
| `scanner/requirements.go` | `RequirementsTxtScanner` — PEP 508 parsing |
| `scanner/pyproject.go` | `PyprojectScanner` — PEP 621 + Poetry, TOML via BurntSushi/toml |
| `scanner/setupcfg.go` | `SetupCfgScanner` — INI-style, install_requires + extras |
| `scanner/pom.go` | `PomScanner` — XML + property resolution, Java 8 compatible |
| `resolver/resolver.go` | npm · Go proxy · PyPI · Maven Central · OSV.dev batch |
| `scorer/scorer.go` | Weighted risk formula |
| `scorer/conflicts.go` | `DetectConflicts` + union-find |
| `advisor/advisor.go` | `Advisor` interface + `StubAdvisor` |
| `advisor/anthropic.go` | `AnthropicAdvisor` — Anthropic API via forced tool use |
| `pipeline/pipeline.go` | Orchestrates all stages, handles git clone |
| `store/store.go` | SQLite persistence |
| `server/server.go` | REST API + SPA handler + URL auto-promotion |
| `cmd/scan.go` | CLI scan subcommand |
| `cmd/serve.go` | CLI serve subcommand |
| `frontend/src/` | React dashboard |
| `web/embed.go` | `//go:embed dist` |

### Dependencies (go.mod)

```
github.com/anthropics/anthropic-sdk-go v1.27.1
github.com/BurntSushi/toml v1.6.0
github.com/Masterminds/semver/v3 v3.2.1
github.com/olekukonko/tablewriter v0.0.5
github.com/spf13/cobra v1.8.1
golang.org/x/mod v0.22.0
modernc.org/sqlite v1.34.4
```

---

## Known gaps / possible next steps

These are open questions, not committed items. No explicit next-step has been agreed with the user yet.

### Score / signal quality
- Cross-repo count is always 0 — the field exists in the model but nothing currently populates it (would need a multi-repo index or a crawl)
- Version-gap scoring treats all major bumps equally; a v1→v2 bump on a library with 2 releases looks the same as log4j 2.14→3.0 with 26 releases
- No differentiation between transitive and direct dependencies in risk scoring

### Coverage gaps
- No `Cargo.toml` (Rust) scanner
- No `build.gradle` / `build.gradle.kts` (Gradle / Kotlin) scanner
- No `composer.json` (PHP) scanner
- No `Package.swift` (Swift) scanner
- `requirements*.txt` glob could miss `requirements/base.txt` style layouts

### Resolver / data quality
- Maven Central version list capped at 200 results; very prolific packages may under-count versions-behind
- PyPI pre-release detection uses `semver.Prerelease() != ""` — packages that don't follow semver (e.g. `1.0.0b1`) may be misclassified
- No retry / back-off on registry errors; a single transient 429 fails the whole dep
- OSV.dev batch size is unbounded — large repos could hit API limits

### UX / polish
- No sorting/filtering controls in the dashboard dep table
- No way to re-run a scan from the detail page
- No export (CSV / JSON) from the dashboard
- No search across scan history
- Scan duration on the list page is computed client-side; clock skew between server and browser can make it wrong

### Observability / ops
- No structured logging on the server (just stderr from the pipeline)
- No health/readiness endpoint (`GET /healthz`)
- SQLite WAL checkpoint never forced; long-running servers accumulate WAL
- No rate limiting on `POST /api/scans` — trivial to flood the server

---

## Environment variables

| Variable | Purpose |
|---|---|
| `ANTHROPIC_API_KEY` | Activates `AnthropicAdvisor` (otherwise stub is used) |
| `GITHUB_TOKEN` | Injected into HTTPS git clone URLs for private repos |
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

### Quick demo fixtures

```bash
# Java / Spring Boot / log4j 2.14.1 (5 CVEs including Log4Shell)
./dep-health scan testdata/java-maven

# React peer-conflict cascade (4 packages that must upgrade together)
./dep-health scan testdata/cascade

# Django 3.2 — 55 CVEs
./dep-health scan testdata/python-pyproject

# Multi-ecosystem: npm + Python + Go
./dep-health scan testdata/mixed
```
