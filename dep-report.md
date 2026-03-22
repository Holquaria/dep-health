# dep-health Dependency Report

> Generated: 2026-03-22
> Scanner: `dep-health scan .` (real project deps only — testdata excluded)
> Advisor: Claude (AnthropicAdvisor)
> Total dependencies: 33 (5 npm · 28 Go)

---

## Summary

| Priority | Count | Ecosystems |
|---|---|---|
| 🔴 Critical — CVEs | 1 | npm (vite) |
| 🟠 High — major version gap | 5 | npm (all), Go (tablewriter, go-strftime) |
| 🟡 Medium — minor/patch behind | 8 | Go |
| 🟢 Up to date | 19 | Go |

Two cascade groups were detected — packages that share peer constraints and **must be upgraded together**:

- **npm-react cascade**: `react`, `react-dom`, `react-router-dom` (all major version bumps)
- **npm-vite cascade**: `vite`, `@vitejs/plugin-react` (includes 11 CVEs on vite)

---

## 🔴 Critical

### `vite` — 5.3.1 → 8.0.1 · 11 CVEs · 172 releases behind

**Risk score: 50 / 100** · cascade with `@vitejs/plugin-react`

Vite 5.x has 11 known CVEs including path traversal, arbitrary file read through `/@fs/` requests, and server-option bypass issues. All are fixed in later releases.

**Key breaking changes across v6, v7, v8:**

- **Node.js ≥18 required** (v6 dropped <18; v7 dropped <20)
- The **CJS build of Vite's Node API is removed** in v6 — any `const vite = require('vite')` in config tooling must become ESM `import`
- `resolve.browserField` option removed in v6
- Legacy plugin hooks (`transformIndexHtml` string return) removed in v7
- `import.meta.env.SSR` semantics tightened in v7

**Migration steps:**

1. Upgrade vite and plugin together (cascade): `npm install vite@8 @vitejs/plugin-react@6`
2. Verify Node.js ≥ 20: `node --version`
3. Update `vite.config.js` if you use CJS `require()` — convert to ESM
4. Run `npm run build` and check for deprecation warnings in output
5. Run `npm run dev` and confirm HMR still works

---

## 🟠 High

### npm-react cascade — `react` + `react-dom` 18.3.1 → 19.2.4 · `react-router-dom` 6.23.1 → 7.13.1

**Risk score: 50 / 100 each** · 426 releases behind (react/react-dom) · 140 releases behind (router)

These three packages form a peer-constraint cascade and should be upgraded in a single pass.

#### React 19 breaking changes

- **`forwardRef` is no longer needed**: refs are now passed as regular props. `React.forwardRef(fn)` still works but will log a deprecation warning. The `cmd/` table components don't use refs, so the dashboard is likely unaffected.
- **Legacy string refs removed** (were already deprecated in 18, now gone)
- **`ReactDOM.render` and `ReactDOM.hydrate` removed** — `react-dom/client` is the only entry point. Our `frontend/src/main.jsx` should already use `createRoot` if it was written for React 18 — double-check.
- **`act` import moves**: `import { act } from 'react'` instead of `react-dom/test-utils`
- New stable APIs: `use()`, `useOptimistic()`, `useFormStatus()`, `useActionState()`

#### React Router v7 breaking changes

- v7 ships in two modes: **library mode** (backward-compatible with v6) and **framework mode** (full Remix-style). Since dep-health uses bare `<Routes>` / `<Route>` / `useParams` etc., **library mode applies** and the upgrade is largely smooth.
- `<Link reloadDocument>` behaviour changes slightly
- `useTransition` renamed to `useNavigation` (was `useNavigationType` in v6 — confirm your usage)
- TypeScript types tightened; route params are now stricter generics

**Migration steps:**

1. `npm install react@19 react-dom@19 react-router-dom@7`
2. In `frontend/src/main.jsx`, confirm `createRoot` is used (not legacy `ReactDOM.render`)
3. Search codebase for `forwardRef` and `useLayoutEffect` — both changed subtly in R19
4. Search for `from 'react-dom/test-utils'` — change to `from 'react'`
5. Run `npm run build` and resolve any TypeScript/JSX type errors
6. Smoke test the dashboard: scan list → scan detail navigation, cascade panel, filter buttons

---

### `github.com/olekukonko/tablewriter` — v0.0.5 → v1.1.4 · 15 releases behind

**Risk score: 45 / 100** · no CVEs · major version bump

tablewriter v1.x is a significant rewrite of the API. The v0.x `NewWriter` / `SetHeader` / `Append` / `Render` pattern is replaced with a builder-style API in v1. This affects `cmd/scan.go` and `cmd/scan_multi.go`.

**Migration steps:**

1. `go get github.com/olekukonko/tablewriter@latest && go mod tidy`
2. Compile: `go build ./...` — the compiler will surface all call-site breakages
3. In `cmd/scan.go` and `cmd/scan_multi.go`, update table construction to the v1 builder API
4. Visually confirm table output still renders correctly: `./dep-health scan testdata/npm-only`

> **Recommendation:** this is the most labour-intensive Go upgrade. Do it in isolation from the npm cascade.

---

### `github.com/ncruces/go-strftime` — v0.1.9 → v1.0.0

**Risk score: 32 / 100** · transitive dep of `modernc.org/sqlite`

This is an indirect dependency. It will be resolved automatically when modernc.org/sqlite is updated via `go mod tidy`.

---

## 🟡 Medium — Go minor updates

These are all backwards-compatible minor/patch updates. They can be batched into a single `go get` + `go mod tidy` pass after the tablewriter migration.

| Package | Current | Latest | Versions behind | Notes |
|---|---|---|---|---|
| `modernc.org/sqlite` | v1.34.4 | v1.47.0 | 29 | Pure-Go SQLite — safe to update; store tests will catch regressions |
| `modernc.org/libc` | v1.55.3 | v1.70.0 | 74 | Indirect dep of sqlite; resolves automatically |
| `golang.org/x/mod` | v0.22.0 | v0.34.0 | 12 | Used in `GoModScanner`; API is stable |
| `golang.org/x/sys` | v0.34.0 | v0.42.0 | 8 | Indirect; OS-level syscall wrappers |
| `github.com/spf13/cobra` | v1.8.1 | v1.10.2 | 5 | CLI framework; no API breaks expected |
| `github.com/Masterminds/semver/v3` | v3.2.1 | v3.4.0 | 3 | Used in conflict detection; API stable |
| `modernc.org/memory` | v1.8.0 | v1.11.0 | 6 | Indirect dep of sqlite |
| `github.com/mattn/go-runewidth` | v0.0.9 | v0.0.21 | 12 | Indirect dep of tablewriter |

**Batch upgrade command:**

```bash
go get \
  modernc.org/sqlite@latest \
  golang.org/x/mod@latest \
  golang.org/x/sys@latest \
  github.com/spf13/cobra@latest \
  github.com/Masterminds/semver/v3@latest
go mod tidy
go test ./...
```

---

## 🟢 Up to date

The following packages are current — no action needed:

`github.com/anthropics/anthropic-sdk-go` · `github.com/BurntSushi/toml` · `github.com/google/uuid` · `github.com/dustin/go-humanize` · `github.com/hashicorp/golang-lru/v2` · `github.com/inconshreveable/mousetrap` · `github.com/tidwall/gjson` · `github.com/tidwall/match` · `github.com/tidwall/pretty` · `github.com/tidwall/sjson` · `github.com/mattn/go-isatty` · `github.com/remyoudompheng/bigfft` · `modernc.org/token` · `modernc.org/strutil` · `modernc.org/gc/v3` · `modernc.org/mathutil` · `github.com/spf13/pflag`

---

## Recommended upgrade order

The cascade constraints mean the order matters. Do not mix npm and Go changes in the same commit.

```
1. [npm]  vite + @vitejs/plugin-react  ← security CVEs, do first
           npm install vite@8 @vitejs/plugin-react@6
           npm run build && npm run dev

2. [npm]  react + react-dom + react-router-dom  ← cascade, single commit
           npm install react@19 react-dom@19 react-router-dom@7
           npm run build
           rebuild Go binary: go build -o dep-health .
           smoke test dashboard

3. [go]   tablewriter v0 → v1  ← API-breaking, isolated commit
           go get github.com/olekukonko/tablewriter@latest && go mod tidy
           fix call sites in cmd/scan.go + cmd/scan_multi.go
           go build ./... && go test ./...
           ./dep-health scan testdata/npm-only  (visual check)

4. [go]   batch minor Go updates  ← safe, single commit
           go get modernc.org/sqlite@latest golang.org/x/mod@latest \
             golang.org/x/sys@latest github.com/spf13/cobra@latest \
             github.com/Masterminds/semver/v3@latest
           go mod tidy && go test ./...
```

Total estimated effort: **~4 hours** — the npm cascade (step 2) and tablewriter API migration (step 3) are the only non-mechanical changes.
