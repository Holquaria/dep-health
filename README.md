# dep-health

A CLI tool and web dashboard that scans repositories for outdated dependencies, scores them by risk, detects peer conflict cascades, and generates upgrade guidance — with optional AI-powered changelog summaries via the Anthropic API.

```
→ Cloning https://github.com/org/repo …
→ Scanning /tmp/dep-health-clone-123 …
  Found 47 dependencies across 2 manifest(s)
→ Resolving versions and checking OSV.dev for CVEs …
→ Scoring and detecting peer conflicts …
→ Generating advisory reports …

+----+-------------------+---------+---------+-------+--------+------------+-------+---------+-------------------------------+
| #  | PACKAGE           | CURRENT | LATEST  | GAP   | BEHIND | CVES       | SCORE | FLAGS   | TOP REASON                    |
+----+-------------------+---------+---------+-------+--------+------------+-------+---------+-------------------------------+
|  1 | lodash            | 4.17.11 | 4.17.21 | patch |     10 | 4 (HIGH)   |  62.0 |         | 4 known CVE(s), highest: HIGH |
|  2 | next              | 13.0.0  | 15.0.0  | major |      8 | -          |  55.0 | CASCADE | major version gap             |
|  3 | react             | 18.2.0  | 19.0.0  | major |      4 | -          |  48.0 | CASCADE | must upgrade with next        |
|  4 | express           | 4.16.0  | 4.19.2  | minor |     18 | 1 (MEDIUM) |  41.0 |         | minor version gap             |
+----+-------------------+---------+---------+-------+--------+------------+-------+---------+-------------------------------+
```

## Features

- **Manifest discovery** — recursively walks a directory, finds `package.json` and `go.mod` files, skips `node_modules`, `.git`, `vendor`
- **Remote scanning** — clones any git repository (`--git-url`) before scanning; no need to check out locally
- **Registry resolution** — fetches the latest version from npm and the Go module proxy; counts published releases in between
- **CVE detection** — queries the [OSV.dev](https://osv.dev) batch API for all discovered packages in a single request
- **Peer conflict detection** — identifies npm peer dependency cascades (packages that must upgrade together) and blocked upgrades (no compatible peer version exists)
- **Risk scoring** — weighted formula across four signals (CVE severity, version gap, release count, cross-repo prevalence) producing a 0–100 score
- **Upgrade guidance** — migration steps and breaking-change warnings for each high-risk dependency
- **AI advisor** — optionally calls the Anthropic API for richer, context-aware changelog summaries (set `ANTHROPIC_API_KEY`)
- **Web dashboard** — `dep-health serve` starts a React dashboard with a REST API; trigger scans, browse history, and inspect results in a browser
- **SQLite persistence** — scan results are stored in a local SQLite database; history survives server restarts
- **JSON output** — `--json` emits machine-readable results for CI pipeline integration

## Installation

Requires **Go 1.22+** and **Node.js 18+** (for the dashboard only).

```bash
git clone https://github.com/your-org/dep-health
cd dep-health
go mod tidy

# Build the frontend (required for the serve command)
cd frontend && npm install && npm run build && cd ..

# Compile the binary (frontend is embedded)
go build -o dep-health .

# Optionally move to PATH
mv dep-health /usr/local/bin/
```

## Quick start

### CLI — local scan

```bash
# Scan a local project
dep-health scan ./my-project

# Show only the 10 highest-risk packages
dep-health scan ./my-project --top 10

# Suppress low-risk noise (score < 30)
dep-health scan ./my-project --min-score 30

# Emit JSON for CI pipelines
dep-health scan ./my-project --json
```

### CLI — remote scan

```bash
# Scan a public GitHub repo (clones with --depth 1, then deletes the clone)
dep-health scan --git-url https://github.com/org/repo

# Scan a private repo (token injected automatically into the clone URL)
GITHUB_TOKEN=ghp_… dep-health scan --git-url https://github.com/org/private-repo

# SSH also works (uses host key agent)
dep-health scan --git-url git@github.com:org/repo.git
```

### Dashboard

```bash
# Start the server (default port 8080)
dep-health serve

# Custom port and database path
dep-health serve --port 9000 --db /var/lib/dep-health/scans.db

# Then open the dashboard
open http://localhost:8080
```

From the dashboard you can trigger scans (local path or Git URL), watch them run in real time, and browse the full dependency table with cascade groups, blocked upgrade warnings, and migration hints.

## CLI reference

### `dep-health scan [directory]`

| Flag | Default | Description |
|---|---|---|
| `--git-url <url>` | `""` | Remote git repository to clone and scan (HTTPS or SSH) |
| `--repo <url>` | `""` | Repository URL attached to every dependency (informational) |
| `--top <n>` | `0` (all) | Limit output to the N highest-risk packages |
| `--min-score <f>` | `0` | Hide packages with a risk score below this threshold (0–100) |
| `--json` | `false` | Emit raw JSON instead of the formatted table |

Either a `directory` argument or `--git-url` must be provided.

### `dep-health serve`

| Flag | Default | Description |
|---|---|---|
| `--port <n>` | `8080` | Port to listen on |
| `--db <path>` | `dep-health.db` | Path to the SQLite database file |

## Output explained

| Column | Meaning |
|---|---|
| **#** | Rank by risk score (1 = highest risk) |
| **Package** | Package name as it appears in the manifest |
| **Current** | Version pinned in the manifest (range operators stripped) |
| **Latest** | Latest stable version according to the registry |
| **Gap** | `patch` / `minor` / `major` — semver distance class |
| **Behind** | Number of published releases between current and latest |
| **CVEs** | Count of known vulnerabilities and highest severity from OSV.dev |
| **Score** | Weighted risk score 0–100 (red ≥ 70, yellow ≥ 40, green < 40) |
| **Flags** | `CASCADE` — must upgrade with other packages; `BLOCKED` — no safe upgrade path |
| **Top Reason** | Primary factor driving the score |

After the table, three additional sections are printed when relevant:

- **Migration hints** — step-by-step upgrade commands for the top 3 riskiest packages
- **Cascade Groups** — packages that share a peer dependency constraint and must be upgraded together
- **Blocked Dependencies** — packages whose upgrade is blocked because no existing version of a required peer satisfies the constraint

## REST API

The `serve` command exposes a simple JSON API.

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/scans` | List all scan runs (newest first) |
| `GET` | `/api/scans/{id}` | Get a scan run and its full dependency report |
| `POST` | `/api/scans` | Trigger a new scan (returns immediately with `202 Accepted`) |

### Trigger a scan

```bash
# Local path
curl -X POST http://localhost:8080/api/scans \
  -H 'Content-Type: application/json' \
  -d '{"dir": "/path/to/repo"}'

# Remote git URL
curl -X POST http://localhost:8080/api/scans \
  -H 'Content-Type: application/json' \
  -d '{"git_url": "https://github.com/org/repo"}'
```

Response: `{"id": 42, "status": "running"}`

Poll `GET /api/scans/42` to check progress; `status` transitions from `running` → `done` (or `failed`).

## Configuration

All configuration is via environment variables — no config file required.

| Variable | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | No | Enables AI-powered changelog summaries via the Anthropic API |
| `GITHUB_TOKEN` | No | Auto-injected into HTTPS clone URLs for private repo scanning |
| `DEP_HEALTH_ORG` | No | Organisation name for future multi-repo scanning |
| `DEP_HEALTH_MAX_CONCURRENCY` | No | Max parallel registry requests (default: 10) |
| `DEP_HEALTH_DB` | No | SQLite database path for the server (default: `dep-health.db`) |

## Risk score formula

```
score = (CVE severity   × 0.40)
      + (version gap    × 0.30)
      + (releases behind × 0.20)
      + (cross-repo count × 0.10)
```

Each factor is normalised to 0–1 before weighting. The final score is multiplied by 100.

| Factor | Max input | Max contribution |
|---|---|---|
| CVE severity | CRITICAL = 1.0, HIGH = 0.8, MEDIUM = 0.5, LOW = 0.2 | 40 pts |
| Version gap | major = 1.0, minor = 0.5, patch = 0.1 | 30 pts |
| Releases behind | ≥ 20 releases → 1.0 | 20 pts |
| Cross-repo count | ≥ 10 repos → 1.0 | 10 pts |

## Adding an ecosystem

The `scanner.Scanner` interface has three methods:

```go
type Scanner interface {
    Name()                                          string
    Matches(path string)                            bool
    Parse(path string, repoURL string) ([]models.Dependency, error)
}
```

Implement those three methods and add the scanner to `scanner.DefaultScanners()`. The resolver, scorer, and advisor pipeline picks it up automatically. You may also need a registry lookup branch in `resolver.resolveOne()` for the new ecosystem.

See `scanner/gomod.go` for the Go module reference implementation.

## Project structure

```
dep-health/
├── main.go            ← entry point
├── cmd/               ← cobra CLI (scan + serve subcommands)
├── pipeline/          ← shared scan pipeline (clone → discover → enrich → score → advise)
├── scanner/           ← manifest discovery and parsing (npm, Go modules)
├── resolver/          ← version lookup (npm, Go proxy) + CVE checks (OSV.dev)
├── scorer/            ← weighted risk formula + peer conflict detection
├── advisor/           ← upgrade guidance (stub + Anthropic interface)
├── store/             ← SQLite persistence (scan runs + dependency reports)
├── server/            ← HTTP REST API + SPA handler
├── web/               ← embedded frontend assets (go:embed)
├── frontend/          ← Vite + React 18 dashboard source
│   └── src/
│       ├── pages/     ← ScanList, ScanDetail
│       └── components/← RiskBadge, DepsTable, CascadePanel, BlockedPanel, …
├── models/            ← shared structs with JSON tags
└── config/            ← env-var configuration loader
```

## Running tests

```bash
go test ./...                # all packages
go test ./scanner/...        # manifest parsing
go test ./resolver/...       # registry + OSV mock server tests
go test ./scorer/...         # conflict detection + scoring
```

## Roadmap

- [ ] `requirements.txt` scanner (PyPI)
- [ ] `pom.xml` (Maven) scanner
- [ ] Real Anthropic API advisor (changelog summarisation)
- [ ] GitHub PR creation with migration guidance
- [ ] Multi-repo scanning with cross-repo prevalence scoring
- [ ] SARIF output for GitHub Advanced Security
