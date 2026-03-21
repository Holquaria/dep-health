# dep-health

A CLI tool that scans repositories for outdated dependencies, scores them by risk, and generates upgrade guidance — with optional AI-powered changelog summaries via the Anthropic API.

```
→ Scanning ./my-app …
  Found 47 dependencies across 2 manifest(s)
→ Resolving versions and checking OSV.dev for CVEs …
→ Scoring dependencies …
→ Generating advisory reports …

+----+-------------------+---------+---------+-------+--------+------------+-------+-------------------------------+
| #  | PACKAGE           | CURRENT | LATEST  | GAP   | BEHIND | CVES       | SCORE | TOP REASON                    |
+----+-------------------+---------+---------+-------+--------+------------+-------+-------------------------------+
|  1 | lodash            | 4.17.11 | 4.17.21 | patch |     10 | 4 (HIGH)   |  62.0 | 4 known CVE(s), highest: HIGH |
|  2 | express           | 4.16.0  | 4.19.2  | minor |     18 | 1 (MEDIUM) |  41.0 | minor version gap             |
|  3 | typescript        | 4.9.0   | 5.4.5   | major |     22 | -          |  35.0 | major version gap             |
+----+-------------------+---------+---------+-------+--------+------------+-------+-------------------------------+
```

## Features

- **Manifest discovery** — recursively walks a directory, finds `package.json` files (more ecosystems coming), and skips `node_modules`, `.git`, `vendor`
- **Registry resolution** — fetches the latest version for each package from the npm registry; counts every published release in between
- **CVE detection** — queries the [OSV.dev](https://osv.dev) batch API in a single request for all discovered packages
- **Risk scoring** — weighted formula across four signals (CVE severity, version gap, release count, cross-repo prevalence) producing a 0–100 score
- **Upgrade guidance** — migration steps and breaking-change warnings for each high-risk dependency
- **AI advisor** — optionally calls the Anthropic API for richer, context-aware changelog summaries (set `ANTHROPIC_API_KEY`)

## Installation

Requires **Go 1.22+**.

```bash
git clone https://github.com/your-org/dep-health
cd dep-health
go mod tidy
go build -o dep-health .
# optionally move to PATH
mv dep-health /usr/local/bin/
```

## Quick start

```bash
# Scan a local Node project
dep-health scan ./my-node-project

# Show only the 10 highest-risk packages
dep-health scan ./my-node-project --top 10

# Suppress low-risk noise (score < 30)
dep-health scan ./my-node-project --min-score 30

# Attach a repo URL to all discovered deps (useful for cross-repo tracking)
dep-health scan ./my-node-project --repo https://github.com/my-org/my-node-project
```

## CLI reference

### `dep-health scan <directory>`

| Flag | Default | Description |
|---|---|---|
| `--repo <url>` | `""` | Repository URL attached to every dependency (informational) |
| `--top <n>` | `0` (all) | Limit output to the N highest-risk packages |
| `--min-score <f>` | `0` | Hide packages with a risk score below this threshold (0–100) |
| `--json` | `false` | Emit raw JSON instead of the formatted table _(not yet wired)_ |

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
| **Top Reason** | Primary factor driving the score |

After the table, migration hints are printed for the top 3 riskiest packages, including step-by-step upgrade commands and breaking-change warnings for major bumps.

## Configuration

All configuration is via environment variables — no config file required.

| Variable | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | No | Enables AI-powered changelog summaries via the Anthropic API |
| `GITHUB_TOKEN` | No | Required for future PR-creation features |
| `DEP_HEALTH_ORG` | No | Organisation name used for future multi-repo scanning |
| `DEP_HEALTH_MAX_CONCURRENCY` | No | Max parallel registry requests (default: 10) |

## Risk score formula

```
score = (CVE severity × 0.40)
      + (version gap   × 0.30)
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
    Name()            string
    Matches(path string) bool
    Parse(path string, repoURL string) ([]models.Dependency, error)
}
```

Implement those three methods for your ecosystem (e.g. `GoModScanner`) and add it to `scanner.DefaultScanners()`. The resolver, scorer, and advisor pipeline picks it up automatically.

See `scanner/scanner.go` for the full `PackageJSONScanner` reference implementation.

## Project structure

```
dep-health/
├── main.go          ← entry point
├── cmd/             ← cobra CLI (root + scan subcommand)
├── scanner/         ← manifest discovery and parsing
├── resolver/        ← version lookup (npm) + CVE checks (OSV.dev)
├── scorer/          ← weighted risk formula
├── advisor/         ← upgrade guidance (stub + Anthropic interface)
├── models/          ← shared structs
└── config/          ← env-var configuration loader
```

## Running tests

```bash
go test ./scanner/...        # unit tests for manifest parsing
go test ./...                # all packages
```

## Roadmap

- [ ] `go.mod` scanner
- [ ] `requirements.txt` scanner
- [ ] `pom.xml` (Maven) scanner
- [ ] Real Anthropic API advisor (changelog summarisation)
- [ ] GitHub PR creation with migration guidance
- [ ] Multi-repo scanning with cross-repo prevalence scoring
- [ ] `--json` output for CI pipeline integration
- [ ] SARIF output for GitHub Advanced Security
