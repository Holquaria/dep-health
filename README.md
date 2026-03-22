# dep-health

A CLI tool and web dashboard that scans repositories for outdated dependencies, scores them by risk, detects peer-conflict cascades, and generates upgrade guidance — with optional AI-powered summaries via the Anthropic API.

```
→ Scanning testdata/java-maven …
→ Found 13 dependencies
→ Resolving versions and checking OSV.dev for CVEs …
→ Scoring and detecting peer conflicts …
→ Generating advisory reports …

+----+-------------------------------------------+-------------+----------+-------+--------+------+-------+
| #  | PACKAGE                                   | CURRENT     | LATEST   | GAP   | BEHIND | CVES | SCORE |
+----+-------------------------------------------+-------------+----------+-------+--------+------+-------+
|  1 | org.apache.logging.log4j:log4j-core       | 2.14.1      | 3.0.0    | major |     26 | 5 () |  50.0 |
|  2 | org.springframework.boot:spring-boot-*    | 2.6.0       | 3.5.3    | major |    100 | 1 () |  50.0 |
|  3 | com.fasterxml.jackson.core:jackson-databind | 2.13.0    | 2.19.0   | minor |     37 | 4 () |  35.0 |
+----+-------------------------------------------+-------------+----------+-------+--------+------+-------+
```

---

## Features

- **Multi-ecosystem scanning** — discovers manifests recursively; skips `node_modules`, `.git`, `vendor`, `.venv`
- **Supported manifest formats:**
  - npm — `package.json` (dependencies + devDependencies)
  - Go — `go.mod` (direct + indirect, local replacements excluded)
  - Python — `requirements.txt`, `pyproject.toml` (PEP 621 + Poetry), `setup.cfg`
  - Java — `pom.xml` (dependencies, dependencyManagement, parent, `${property}` resolution)
- **Remote scanning** — `--git-url` clones any repo with `--depth 1` then removes the clone
- **CVE detection** — single batch query to [OSV.dev](https://osv.dev) for all discovered packages
- **Registry resolution** — npm registry, Go module proxy, PyPI JSON API, Maven Central search API
- **LTS-aware versioning** — tracks `LatestInMajor` alongside absolute latest; scoring reduces urgency when current major is up to date
- **Risk scoring** — weighted formula across CVE severity, version gap, release count, cross-repo prevalence
- **Peer conflict detection** — cascade groups (must upgrade together) and blocked upgrades
- **License detection** — SPDX license extraction from npm/PyPI registries with permissive/copyleft/unknown classification
- **Upgrade guidance** — ecosystem-specific migration steps and breaking-change warnings; two-phase upgrade for cross-major bumps
- **AI advisor** — optional Anthropic API integration for richer changelog summaries (set `ANTHROPIC_API_KEY`)
- **Web dashboard** — React SPA with real-time scan progress, dep table, cascade panel, migration hints, license badges
- **Anonymous sessions** — cookie-based session isolation; each browser gets its own scan history with no login required
- **SQLite persistence** — scan history survives server restarts
- **JSON output** — `--json` flag for CI pipeline consumption

---

## Prerequisites

| Tool | Version | Required for |
|---|---|---|
| Go | 1.22+ | building and running dep-health |
| Node.js | 18+ | building the web dashboard |
| npm | 8+ | installing frontend dependencies |
| git | any | `--git-url` remote scanning |

---

## Running locally

### 1. Clone the repository

```bash
git clone https://github.com/your-org/dep-health
cd dep-health
```

### 2. Install Go dependencies

```bash
go mod tidy
```

### 3. Build the frontend

The dashboard is a Vite + React app that gets embedded into the Go binary at build time via `//go:embed`. **You must build it before compiling the binary** — otherwise `dep-health serve` will serve an empty UI.

```bash
cd frontend
npm install
npm run build    # outputs to ../web/dist/
cd ..
```

### 4. Compile the binary

```bash
go build -o dep-health .
```

### 5. Scan something

The quickest way to see results is to scan one of the included test fixtures — no network setup required beyond the registry lookups:

```bash
# Java 8 / Spring Boot 2.6 + log4j 2.14.1 (Log4Shell — 5 CVEs)
./dep-health scan testdata/java-maven

# React peer-conflict cascade (4 packages that must upgrade together)
./dep-health scan testdata/cascade

# Multi-ecosystem: npm + Python + Go in one directory
./dep-health scan testdata/mixed

# Python pyproject.toml — Django 3.2 with 55 CVEs
./dep-health scan testdata/python-pyproject
```

Or scan a real project:

```bash
./dep-health scan /path/to/your/project
./dep-health scan --git-url https://github.com/expressjs/express
./dep-health scan --git-url https://github.com/pallets/flask
```

### 6. Start the dashboard

```bash
./dep-health serve            # default port 8080
./dep-health serve --port 9000 --db ./my-scans.db

open http://localhost:8080    # macOS; use xdg-open on Linux
```

From the dashboard you can trigger scans by local path or Git URL, watch progress in real time, and browse the full dep table with cascade groups, blocked upgrades, and migration hints.

---

## Development workflow

### Frontend hot-reload

Vite is configured to proxy `/api/*` to port 8080, so you can develop the frontend against a live scan server with hot module replacement:

```bash
# Terminal 1 — Go server
./dep-health serve

# Terminal 2 — Vite dev server (http://localhost:5173)
cd frontend && npm run dev
```

### Run the full test suite

```bash
go test ./...                  # all packages (~50 tests)
go test ./scanner/...          # manifest parsing (npm, Go, Python, Java)
go test ./resolver/...         # registry + OSV mock server tests
go test ./scorer/...           # conflict detection + risk scoring
go test ./advisor/...          # advisory stub (deterministic output)
```

### Enable the AI advisor

```bash
export ANTHROPIC_API_KEY=sk-ant-…
./dep-health scan testdata/java-maven
```

The pipeline selects `AnthropicAdvisor` automatically when `ANTHROPIC_API_KEY` is set. Without it the deterministic `StubAdvisor` runs, so development and CI work without any API key.

---

## Test fixtures

`testdata/` contains controlled manifest files with intentionally old pinned versions — no real source code. Use them for development, demos, and regression testing without needing an external repo.

| Fixture | Ecosystems | Highlights |
|---|---|---|
| `testdata/npm-only` | npm | lodash@3, axios@0.21 — CVEs, babel+webpack cascade group |
| `testdata/python-only` | requirements.txt | Flask@1.1, Werkzeug@1 — 13 CVEs on Werkzeug |
| `testdata/python-pyproject` | pyproject.toml (PEP 621) | Django@3.2 — 55 known CVEs |
| `testdata/python-setupcfg` | setup.cfg | Flask + SQLAlchemy, extras_require sections |
| `testdata/java-maven` | pom.xml | Spring Boot 2.6 / Java 8, log4j 2.14.1 pre-Log4Shell (5 CVEs) |
| `testdata/mixed` | npm + Python + Go | Multi-ecosystem in a single directory |
| `testdata/cascade` | npm | React@16 — 4-package cascade (react, react-dom, react-router-dom, @testing-library/react) |
| `testdata/no-deps` | — | Empty manifests — graceful-empty handling |

---

## CLI reference

### `dep-health scan [directory]`

Either `directory` or `--git-url` must be provided.

| Flag | Default | Description |
|---|---|---|
| `--git-url <url>` | — | Remote repo to clone and scan (HTTPS or SSH) |
| `--repo <url>` | — | Repository URL to attach to each dep (informational) |
| `--top <n>` | 0 (all) | Show only the N highest-risk packages |
| `--min-score <f>` | 0 | Hide packages with a risk score below this threshold |
| `--json` | false | Emit `[]AdvisoryReport` as indented JSON instead of the table |

### `dep-health serve`

| Flag | Default | Description |
|---|---|---|
| `--port <n>` | 8080 | HTTP port to listen on |
| `--db <path>` | `dep-health.db` | SQLite database path |

---

## Environment variables

| Variable | Description |
|---|---|
| `ANTHROPIC_API_KEY` | Activates AI-powered upgrade summaries; omit to use the stub advisor |
| `GITHUB_TOKEN` | Injected into HTTPS clone URLs for private repository access |
| `DEP_HEALTH_MAX_CONCURRENCY` | Max parallel registry requests (default: `10`) |
| `DEP_HEALTH_DB` | SQLite database path for `serve` (default: `dep-health.db`) |

---

## Output explained

| Column | Meaning |
|---|---|
| **#** | Rank by risk score (1 = highest risk) |
| **Package** | Name as in the manifest; Maven uses `groupId:artifactId` |
| **Current** | Version pinned in the manifest (range operators stripped) |
| **Latest** | Latest stable release from the registry |
| **Major Latest** | Newest version within the same major line (shown only when it differs from Latest) |
| **Gap** | `patch` / `minor` / `major` — semver distance class |
| **Behind** | Published releases between current and latest |
| **CVEs** | Count of known vulnerabilities from OSV.dev |
| **Score** | Risk score 0–100 (red ≥ 70, amber ≥ 40, green < 40) |
| **Flags** | `CASCADE` — must upgrade with peers; `BLOCKED` — no safe upgrade path exists |
| **Top Reason** | Primary factor driving the score |

Below the table, three sections appear when applicable:

- **Migration hints** — ecosystem-specific upgrade commands for the top 3 riskiest packages, with breaking-change warnings for major bumps and a CVE verification reminder
- **Cascade Groups** — packages that share a peer constraint and must all be upgraded together
- **Blocked Dependencies** — packages where the latest version requires a peer that no current release satisfies

---

## REST API

The `serve` command exposes a JSON API on the configured port.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/scans` | List scan runs for the current session, newest first |
| `GET` | `/api/scans/{id}` | Scan run metadata + full dependency report (404 if not your session) |
| `POST` | `/api/scans` | Trigger a new async scan (returns `202 Accepted` immediately) |
| `POST` | `/api/scans/multi` | Trigger a multi-repo scan (`{"targets": ["url1", "url2"]}`, min 2) |

All API routes set a `dep_health_session` cookie on first request. Each browser session sees only its own scans — no login required.

```bash
# Trigger a local scan
curl -s -c cookies.txt -X POST http://localhost:8080/api/scans \
  -H 'Content-Type: application/json' \
  -d '{"dir": "/path/to/project"}'
# → {"id": 3, "status": "running"}

# Trigger a remote scan
curl -s -b cookies.txt -X POST http://localhost:8080/api/scans \
  -H 'Content-Type: application/json' \
  -d '{"git_url": "https://github.com/org/repo"}'

# Poll for completion (use same cookie jar to stay in session)
curl -s -b cookies.txt http://localhost:8080/api/scans/3 | jq .status
# "running" → "done" | "failed"
```

---

## Risk score formula

```
score = (CVE severity     × 0.40)
      + (version gap      × 0.30 × ltsAwareFactor)
      + (releases behind  × 0.20)
      + (cross-repo count × 0.10)
```

Each factor normalised to 0–1 before weighting. Final score multiplied by 100. The `ltsAwareFactor` reduces version-gap urgency to 0.3 when the user is current within their major line but a newer major exists (LTS scenario).

| Factor | Scale | Max contribution |
|---|---|---|
| CVE severity | CRITICAL=1.0, HIGH=0.8, MEDIUM=0.5, LOW=0.2 | 40 pts |
| Version gap | major=1.0, minor=0.5, patch=0.1 | 30 pts |
| Releases behind | ≥ 20 releases → 1.0 | 20 pts |
| Cross-repo count | ≥ 10 repos → 1.0 | 10 pts |

---

## Adding an ecosystem

Implement the `Scanner` interface and register it in `DefaultScanners()`:

```go
type MyScanner struct{}

func (s *MyScanner) Name() string              { return "myecosystem/manifest.ext" }
func (s *MyScanner) Matches(path string) bool  { return filepath.Base(path) == "manifest.ext" }
func (s *MyScanner) Parse(path, repoURL string) ([]models.Dependency, error) { … }
```

Add a `case "myecosystem":` branch in `resolver.resolveOne()` pointing at the registry. The scorer, advisor, and pipeline pick everything else up automatically.

Reference implementations: `scanner/gomod.go` (Go), `scanner/pom.go` (XML + property resolution), `scanner/pyproject.go` (TOML + two format variants).

---

## Project structure

```
dep-health/
├── main.go                    entry point
├── cmd/                       cobra CLI (scan + serve subcommands)
├── config/                    env-var loader
├── models/                    shared structs (Dependency → Enriched → Scored → Advisory)
│
├── scanner/                   manifest parsers
│   ├── scanner.go             Scanner interface + Discover() + PackageJSONScanner
│   ├── gomod.go               Go modules
│   ├── requirements.go        Python requirements.txt
│   ├── pyproject.go           Python pyproject.toml (PEP 621 + Poetry)
│   ├── setupcfg.go            Python setup.cfg
│   └── pom.go                 Java pom.xml
│
├── resolver/
│   └── resolver.go            npm · Go proxy · PyPI · Maven Central · OSV.dev batch
│
├── scorer/
│   ├── scorer.go              weighted risk formula
│   └── conflicts.go           peer conflict + cascade group detection
│
├── advisor/
│   ├── advisor.go             Advisor interface + StubAdvisor
│   └── anthropic.go           AnthropicAdvisor (activated by ANTHROPIC_API_KEY)
│
├── pipeline/                  orchestrates all stages; handles git clone
├── store/                     SQLite persistence
├── server/                    HTTP REST API + SPA fallback
├── web/                       go:embed wrapper for compiled frontend
│
├── frontend/                  Vite + React 18 dashboard source
│   └── src/
│       ├── pages/             ScanList, ScanDetail
│       └── components/        DepsTable, CascadePanel, BlockedPanel, RiskBadge, …
│
└── testdata/                  fixture manifests for local testing and demos
    ├── npm-only/              package.json
    ├── python-only/           requirements.txt
    ├── python-pyproject/      pyproject.toml (PEP 621)
    ├── python-setupcfg/       setup.cfg
    ├── java-maven/            pom.xml (Java 8 + Spring Boot 2.6 + log4j 2.14.1)
    ├── mixed/                 package.json + requirements.txt + go.mod
    ├── cascade/               npm peer-conflict scenario (React ecosystem)
    └── no-deps/               empty manifests
```
