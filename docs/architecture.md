# dep-health — Architecture

This document covers the internal design of dep-health: package responsibilities, data flow, concurrency model, the risk-scoring formula, and the extension points for adding new ecosystems and advisors.

---

## Table of contents

1. [Package dependency graph](#1-package-dependency-graph)
2. [End-to-end pipeline](#2-end-to-end-pipeline)
3. [Data model](#3-data-model)
4. [Scanner package](#4-scanner-package)
5. [Resolver package — concurrency model](#5-resolver-package--concurrency-model)
6. [Scorer package — risk formula](#6-scorer-package--risk-formula)
7. [Advisor package](#7-advisor-package)
8. [External API contracts](#8-external-api-contracts)
9. [Extension guide](#9-extension-guide)

---

## 1. Package dependency graph

The dependency graph is strictly layered — no import cycles are possible.

```mermaid
graph TD
    main["main.go"]
    cmd["cmd/\n(cobra CLI)"]
    scanner["scanner/\n(manifest parsing)"]
    resolver["resolver/\n(registry + CVE lookup)"]
    scorer["scorer/\n(risk scoring)"]
    advisor["advisor/\n(upgrade guidance)"]
    models["models/\n(shared structs)"]
    config["config/\n(env vars)"]

    main --> cmd
    cmd --> scanner
    cmd --> resolver
    cmd --> scorer
    cmd --> advisor
    cmd --> config

    scanner --> models
    resolver --> models
    scorer --> models
    advisor --> models

    style models fill:#f0f4ff,stroke:#6c8ebf
    style config fill:#fff4e0,stroke:#d79b00
```

**Rule:** only `cmd/` may import all other packages. Domain packages (`scanner`, `resolver`, `scorer`, `advisor`) import only `models`. No domain package imports another domain package.

---

## 2. End-to-end pipeline

The `dep-health scan <dir>` command runs five sequential stages. Registry lookups inside Stage 2 are parallelised; everything else is single-threaded.

```mermaid
sequenceDiagram
    autonumber
    actor User
    participant cmd as cmd/scan.go
    participant scanner as scanner.Discover()
    participant resolver as resolver.Enrich()
    participant npm as registry.npmjs.org
    participant osv as api.osv.dev
    participant scorer as scorer.Score()
    participant advisor as advisor.Advise()

    User->>cmd: dep-health scan ./my-app

    cmd->>scanner: Discover(dir, repoURL, scanners)
    scanner-->>cmd: []Dependency (N packages)

    cmd->>resolver: Enrich(ctx, deps)
    note over resolver: spawns up to 10 goroutines<br/>one per dependency
    par concurrent registry lookups
        resolver->>npm: GET /lodash
        npm-->>resolver: {dist-tags, versions}
        resolver->>npm: GET /express
        npm-->>resolver: {dist-tags, versions}
    end
    note over resolver: all goroutines finish (WaitGroup)
    resolver->>osv: POST /v1/querybatch (all deps, one request)
    osv-->>resolver: [{vulns:[...]}, ...]
    resolver-->>cmd: []EnrichedDependency

    cmd->>scorer: Score(enriched, crossRepoCounts)
    scorer-->>cmd: []ScoredDependency (sorted desc by RiskScore)

    loop for each ScoredDependency
        cmd->>advisor: Advise(ctx, dep)
        advisor-->>cmd: AdvisoryReport
    end

    cmd->>User: colour-coded table + migration hints
```

---

## 3. Data model

Each pipeline stage produces a richer struct by embedding the previous one. This keeps the type system honest — you can never pass a `ScoredDependency` where a raw `Dependency` is expected, but you can always access the raw fields through the embedding chain.

```mermaid
classDiagram
    class Dependency {
        +string Name
        +string CurrentVersion
        +string Ecosystem
        +string ManifestPath
        +string RepoURL
    }

    class Vulnerability {
        +string ID
        +string Severity
        +string Summary
        +string URL
    }

    class EnrichedDependency {
        +string LatestVersion
        +string SeverityGap
        +int VersionsBehind
        +[]Vulnerability Vulnerabilities
    }

    class ScoredDependency {
        +float64 RiskScore
        +int CrossRepoCount
        +[]string Reasons
    }

    class AdvisoryReport {
        +string Summary
        +[]string BreakingChanges
        +[]string MigrationSteps
        +string PRUrl
    }

    EnrichedDependency --|> Dependency : embeds
    EnrichedDependency "1" o-- "0..*" Vulnerability : contains
    ScoredDependency --|> EnrichedDependency : embeds
    AdvisoryReport --|> ScoredDependency : embeds
```

**Embedding chain:** `Dependency` → `EnrichedDependency` → `ScoredDependency` → `AdvisoryReport`

Every struct exposes all fields of its ancestors via Go struct embedding. Callers that only need version data work with `EnrichedDependency`; the full `AdvisoryReport` is only materialised at the very end.

---

## 4. Scanner package

The `Scanner` interface is the single extension point for manifest parsers.

```mermaid
classDiagram
    class Scanner {
        <<interface>>
        +Name() string
        +Matches(path string) bool
        +Parse(path, repoURL string) []Dependency
    }

    class PackageJSONScanner {
        +Name() string
        +Matches(path string) bool
        +Parse(path, repoURL string) []Dependency
    }

    class GoModScanner {
        <<planned>>
        +Name() string
        +Matches(path string) bool
        +Parse(path, repoURL string) []Dependency
    }

    class RequirementsTxtScanner {
        <<planned>>
        +Name() string
        +Matches(path string) bool
        +Parse(path, repoURL string) []Dependency
    }

    Scanner <|.. PackageJSONScanner : implements
    Scanner <|.. GoModScanner : implements
    Scanner <|.. RequirementsTxtScanner : implements
```

### Discovery walk

```mermaid
flowchart TD
    A([Start: Discover dir]) --> B{filepath.Walk entry}
    B --> C{Is directory?}
    C -- yes --> D{Skipped name?\nnode_modules .git vendor}
    D -- yes --> E[filepath.SkipDir]
    D -- no --> B
    C -- no --> F{Scanner.Matches path?}
    F -- no → next scanner --> F
    F -- yes --> G[Scanner.Parse path]
    G --> H{Error?}
    H -- yes --> I[log warning, continue]
    H -- no --> J[append to deps]
    J --> B
    E --> B
    B --> K([Return deps])
```

### Version string normalisation

Range operators are stripped from version specifiers before any semver comparison occurs. The logic lives in `cleanVersion()`.

```mermaid
flowchart LR
    A["raw version\ne.g. ^4.17.11"] --> B{Protocol prefix?\nworkspace: file: link:}
    B -- yes --> C[discard — return empty]
    B -- no --> D[strip ^~>=<* with regex]
    D --> E{Range expression?\ne.g. '>=1.0 <2.0'}
    E -- yes --> F[take first token]
    E -- no --> G[trim whitespace]
    F --> G
    G --> H[bare semver\ne.g. 4.17.11]
```

---

## 5. Resolver package — concurrency model

The resolver uses two distinct parallelism strategies to maximise throughput while being respectful to external APIs.

```mermaid
flowchart TD
    A([Enrich called with N deps]) --> B[Create semaphore\nchan size = Concurrency]

    B --> C[Launch N goroutines]

    subgraph goroutines ["goroutines (up to Concurrency run at once)"]
        direction LR
        G1["goroutine 1\nresolveNPM(dep[0])"]
        G2["goroutine 2\nresolveNPM(dep[1])"]
        GN["goroutine N\nresolveNPM(dep[N])"]
    end

    C --> goroutines
    goroutines --> D[WaitGroup.Wait — all finish]
    D --> E["enrichVulnerabilities()\none batch POST to OSV.dev"]
    E --> F([Return []EnrichedDependency])
```

**Why two phases?**

- Registry lookups (npm, PyPI, etc.) are independent per package and benefit maximally from parallelism. A semaphore cap (`defaultConcurrency = 10`) prevents connection exhaustion.
- OSV.dev accepts a batch request — sending all packages in one POST is cheaper than N individual calls and avoids hitting rate limits.

### Semaphore pattern

```mermaid
sequenceDiagram
    participant g as goroutine i
    participant sem as sem (chan struct{}, cap=10)
    participant api as registry API

    g->>sem: sem ← struct{}{} (blocks if 10 others running)
    sem-->>g: acquired
    g->>api: GET /package-name
    api-->>g: response
    g->>sem: ←sem (release slot)
```

---

## 6. Scorer package — risk formula

Each dependency is scored across four independent signals. Scores are normalised to 0–1 per factor, then combined using fixed weights.

```mermaid
flowchart LR
    dep([EnrichedDependency])

    dep --> F1
    dep --> F2
    dep --> F3
    dep --> F4

    subgraph factors ["Factor calculation (each → 0.0 – 1.0)"]
        F1["CVE severity factor\nCRITICAL=1.0 HIGH=0.8\nMEDIUM=0.5 LOW=0.2"]
        F2["Version gap factor\nmajor=1.0 minor=0.5 patch=0.1"]
        F3["Versions-behind factor\nbehind / 20  (capped at 1.0)"]
        F4["Cross-repo factor\ncount / 10  (capped at 1.0)"]
    end

    F1 --> W1["× 0.40"]
    F2 --> W2["× 0.30"]
    F3 --> W3["× 0.20"]
    F4 --> W4["× 0.10"]

    W1 & W2 & W3 & W4 --> SUM["sum × 100"]
    SUM --> score(["RiskScore 0–100"])
```

### Score band interpretation

| Score | Band | Colour | Typical profile |
|---|---|---|---|
| 70–100 | Critical | Red / bold | CRITICAL or HIGH CVE present |
| 40–69 | Elevated | Yellow | Major version lag or HIGH CVE |
| 0–39 | Low | Green | Patch/minor lag, no CVEs |

### Sorting

`scorer.Score()` returns `[]ScoredDependency` sorted descending by `RiskScore`. The table always shows the highest-risk package first regardless of the order dependencies appear in the manifest.

---

## 7. Advisor package

The advisor is designed for easy swapping between the stub and a real API-backed implementation.

```mermaid
classDiagram
    class Advisor {
        <<interface>>
        +Advise(ctx, ScoredDependency) AdvisoryReport
    }

    class StubAdvisor {
        +Advise(ctx, ScoredDependency) AdvisoryReport
    }

    class AnthropicAdvisor {
        -string apiKey
        +Advise(ctx, ScoredDependency) AdvisoryReport
    }

    Advisor <|.. StubAdvisor : implements
    Advisor <|.. AnthropicAdvisor : implements
```

### Selection logic in `cmd/scan.go`

```mermaid
flowchart LR
    A{ANTHROPIC_API_KEY set?}
    A -- yes --> B[advisor.NewAnthropic]
    A -- no  --> C[advisor.NewStub]
    B & C --> D[advisor.Advisor interface]
    D --> E["Advise(ctx, dep) → AdvisoryReport"]
```

### Stub output

The stub generates deterministic guidance from metadata alone — no network calls:

- **Summary** — `"Upgrade {name} from {current} to {latest}. Risk score: {n}/100."`
- **BreakingChanges** — populated only for major-version gaps
- **MigrationSteps** — ecosystem-appropriate shell commands (`npm install`, `go get`, `pip install --upgrade`)

### AnthropicAdvisor (planned)

When implemented, `AnthropicAdvisor.Advise` will:

1. Build a prompt including the package name, version delta, CVE summaries, and changelog URL
2. Call `POST /v1/messages` with `claude-sonnet-4-6` (or configurable model)
3. Parse the response into `Summary`, `BreakingChanges`, and `MigrationSteps`
4. Return the populated `AdvisoryReport`

Wire it up using the `/claude-api` skill or the `github.com/anthropics/anthropic-sdk-go` package.

---

## 8. External API contracts

### npm Registry

| | |
|---|---|
| **Endpoint** | `GET https://registry.npmjs.org/{package}` |
| **Auth** | None required for public packages |
| **Used fields** | `dist-tags.latest` (latest stable version), `versions` map (all published versions for counting) |
| **Rate limit** | Informal; 10-connection semaphore keeps dep-health well within normal bounds |

### OSV.dev Batch API

| | |
|---|---|
| **Endpoint** | `POST https://api.osv.dev/v1/querybatch` |
| **Auth** | None |
| **Request** | `{"queries":[{"package":{"name":"lodash","ecosystem":"npm"},"version":"4.17.11"},…]}` |
| **Response** | `{"results":[{"vulns":[{"id":"GHSA-…","summary":"…","database_specific":{"severity":"HIGH"},…}]}]}` |
| **Alignment** | `results[i]` corresponds exactly to `queries[i]` — index alignment is preserved |
| **Severity source** | Prefer `database_specific.severity`; fall back to `severity[0].score` (CVSS string) |

---

## 9. Extension guide

### Adding a new ecosystem scanner

1. Create a file in `scanner/`, e.g. `scanner/gomod.go`
2. Implement the three `Scanner` interface methods
3. Register it in `DefaultScanners()`

```go
// scanner/gomod.go
package scanner

import "dep-health/models"

type GoModScanner struct{}

func (s *GoModScanner) Name() string               { return "go/go.mod" }
func (s *GoModScanner) Matches(path string) bool   { return filepath.Base(path) == "go.mod" }
func (s *GoModScanner) Parse(path, repoURL string) ([]models.Dependency, error) {
    // parse go.mod, return []models.Dependency with Ecosystem: "go"
}
```

```go
// scanner/scanner.go — DefaultScanners
func DefaultScanners() []Scanner {
    return []Scanner{
        &PackageJSONScanner{},
        &GoModScanner{},         // add here
    }
}
```

The resolver, scorer, and advisor pipeline picks it up automatically. You may also need to add a registry lookup branch in `resolver.resolveOne()` for the new ecosystem.

### Adding a new registry lookup

In `resolver/resolver.go`, extend `resolveOne()`:

```go
func (r *Resolver) resolveOne(ctx context.Context, dep models.Dependency) (models.EnrichedDependency, error) {
    switch dep.Ecosystem {
    case "npm":
        return r.resolveNPM(ctx, dep)
    case "go":
        return r.resolveGoProxy(ctx, dep)   // add your implementation
    default:
        return models.EnrichedDependency{Dependency: dep}, nil
    }
}
```

### Wiring the real Anthropic advisor

```go
// advisor/anthropic.go
func (a *AnthropicAdvisor) Advise(ctx context.Context, dep models.ScoredDependency) (models.AdvisoryReport, error) {
    client := anthropic.NewClient(a.apiKey)

    prompt := buildPrompt(dep) // construct from dep metadata + CVE summaries

    msg, err := client.Messages.New(ctx, anthropic.MessageNewParams{
        Model:     anthropic.F(anthropic.ModelClaudeSonnet4_6),
        MaxTokens: anthropic.F(int64(1024)),
        Messages: anthropic.F([]anthropic.MessageParam{
            anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
        }),
    })
    if err != nil {
        return models.AdvisoryReport{}, err
    }

    return parseAnthropicResponse(dep, msg.Content[0].Text), nil
}
```

See the `/claude-api` skill for a complete working example with the Anthropic Go SDK.
