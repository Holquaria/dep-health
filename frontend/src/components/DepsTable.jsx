import React, { useState, useMemo, useEffect } from 'react'
import RiskBadge from './RiskBadge.jsx'
import { cascadeColor } from './cascadeColor.js'

const GAP_ORDER = { major: 3, minor: 2, patch: 1 }

function severityLabel(score) {
  if (score >= 70) return 'critical'
  if (score >= 50) return 'high'
  if (score >= 25) return 'medium'
  return 'low'
}

function Arrow({ sortKey, col, dir }) {
  if (sortKey !== col) return <span style={{ marginLeft: 3, opacity: 0.3, fontSize: 9 }}>⬍</span>
  return <span style={{ marginLeft: 3, fontSize: 9 }}>{dir === 'asc' ? '▲' : '▼'}</span>
}

function SortTh({ col, sortKey, sortDir, onSort, children, style }) {
  return (
    <th
      style={{ cursor: 'pointer', userSelect: 'none', ...style }}
      onClick={() => onSort(col)}
    >
      {children}
      <Arrow sortKey={sortKey} col={col} dir={sortDir} />
    </th>
  )
}

function Chip({ label, active, onClick }) {
  return (
    <button
      className={`btn ${active ? 'btn-primary' : ''}`}
      style={{ padding: '2px 10px', fontSize: 11 }}
      onClick={onClick}
    >
      {label}
    </button>
  )
}

export default function DepsTable({
  deps,
  showRepo = false,
  repos = [],
  repoFilter = null,
  setRepoFilter = null,
}) {
  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [ecosystem, setEcosystem] = useState('all')
  const [severity, setSeverity] = useState('all')
  const [hasCves, setHasCves] = useState(false)
  const [cascadeOnly, setCascadeOnly] = useState(false)
  const [sortKey, setSortKey] = useState('score')
  const [sortDir, setSortDir] = useState('desc')

  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(search), 150)
    return () => clearTimeout(t)
  }, [search])

  function handleSort(col) {
    if (sortKey === col) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortKey(col)
      setSortDir(col === 'name' ? 'asc' : 'desc')
    }
  }

  const allDeps = deps ?? []

  // derive unique ecosystems present in data
  const ecosystems = useMemo(
    () => [...new Set(allDeps.map(d => d.ecosystem).filter(Boolean))].sort(),
    [allDeps]
  )

  const filtered = useMemo(() => {
    let list = allDeps

    if (repoFilter) {
      list = list.filter(d => d.repo_source === repoFilter)
    }

    if (debouncedSearch.trim()) {
      const q = debouncedSearch.trim().toLowerCase()
      list = list.filter(d => d.name?.toLowerCase().includes(q))
    }

    if (ecosystem !== 'all') {
      list = list.filter(d => d.ecosystem === ecosystem)
    }

    if (severity !== 'all') {
      list = list.filter(d => severityLabel(d.risk_score ?? 0) === severity)
    }

    if (hasCves) {
      list = list.filter(d => (d.vulnerabilities?.length ?? 0) > 0)
    }

    if (cascadeOnly) {
      list = list.filter(d => !!d.cascade_group)
    }

    return list
  }, [allDeps, repoFilter, debouncedSearch, ecosystem, severity, hasCves, cascadeOnly])

  const sorted = useMemo(() => {
    const list = [...filtered]
    list.sort((a, b) => {
      let cmp = 0
      if (sortKey === 'score')  cmp = (a.risk_score ?? 0) - (b.risk_score ?? 0)
      if (sortKey === 'name')   cmp = (a.name ?? '').localeCompare(b.name ?? '')
      if (sortKey === 'gap')    cmp = (GAP_ORDER[a.severity_gap] ?? 0) - (GAP_ORDER[b.severity_gap] ?? 0)
      if (sortKey === 'cves')   cmp = (a.vulnerabilities?.length ?? 0) - (b.vulnerabilities?.length ?? 0)
      if (sortKey === 'behind') cmp = (a.versions_behind ?? 0) - (b.versions_behind ?? 0)
      return sortDir === 'asc' ? cmp : -cmp
    })
    return list
  }, [filtered, sortKey, sortDir])

  if (allDeps.length === 0) {
    return <p className="empty">No dependencies recorded for this scan.</p>
  }

  const severityOptions = ['critical', 'high', 'medium', 'low']

  return (
    <div>
      {/* ── Filter bar ── */}
      <div className="card" style={{ marginBottom: 8, padding: 12 }}>
        {/* Row 1: search + toggles */}
        <div className="flex gap-8 items-center" style={{ flexWrap: 'wrap', marginBottom: 8 }}>
          <input
            className="input"
            style={{ maxWidth: 220 }}
            placeholder="Search package name…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <label className="flex gap-8 items-center" style={{ cursor: 'pointer', fontSize: 12, gap: 6 }}>
            <input
              type="checkbox"
              checked={hasCves}
              onChange={e => setHasCves(e.target.checked)}
            />
            Has CVEs
          </label>
          <label className="flex gap-8 items-center" style={{ cursor: 'pointer', fontSize: 12, gap: 6 }}>
            <input
              type="checkbox"
              checked={cascadeOnly}
              onChange={e => setCascadeOnly(e.target.checked)}
            />
            Cascade only
          </label>
        </div>

        {/* Row 2: ecosystem chips */}
        {ecosystems.length > 1 && (
          <div className="flex gap-8 items-center" style={{ flexWrap: 'wrap', marginBottom: 8 }}>
            <span className="text-muted" style={{ fontSize: 11, minWidth: 70 }}>ECOSYSTEM</span>
            <Chip label="All" active={ecosystem === 'all'} onClick={() => setEcosystem('all')} />
            {ecosystems.map(eco => (
              <Chip key={eco} label={eco} active={ecosystem === eco} onClick={() => setEcosystem(eco)} />
            ))}
          </div>
        )}

        {/* Row 3: severity chips */}
        <div className="flex gap-8 items-center" style={{ flexWrap: 'wrap', marginBottom: 8 }}>
          <span className="text-muted" style={{ fontSize: 11, minWidth: 70 }}>SEVERITY</span>
          <Chip label="All" active={severity === 'all'} onClick={() => setSeverity('all')} />
          {severityOptions.map(s => (
            <Chip
              key={s}
              label={s.charAt(0).toUpperCase() + s.slice(1)}
              active={severity === s}
              onClick={() => setSeverity(s)}
            />
          ))}
        </div>

        {/* Row 4: repo chips (multi-repo only) */}
        {repos.length > 0 && setRepoFilter && (
          <div className="flex gap-8 items-center" style={{ flexWrap: 'wrap', marginBottom: 8 }}>
            <span className="text-muted" style={{ fontSize: 11, minWidth: 70 }}>REPO</span>
            <Chip
              label={`All (${allDeps.length})`}
              active={!repoFilter}
              onClick={() => setRepoFilter(null)}
            />
            {repos.map(repo => {
              const count = allDeps.filter(d => d.repo_source === repo).length
              return (
                <Chip
                  key={repo}
                  label={`${repo} (${count})`}
                  active={repoFilter === repo}
                  onClick={() => setRepoFilter(repoFilter === repo ? null : repo)}
                />
              )
            })}
          </div>
        )}

        {/* Result count */}
        <div className="text-muted" style={{ fontSize: 12 }}>
          Showing {sorted.length} of {allDeps.length} {allDeps.length === 1 ? 'dependency' : 'dependencies'}
        </div>
      </div>

      {sorted.length === 0 ? (
        <p className="empty">No dependencies match the current filters.</p>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th style={{ width: 3, padding: 0 }} />
                <th>#</th>
                {showRepo && <th>Repo</th>}
                <SortTh col="name" sortKey={sortKey} sortDir={sortDir} onSort={handleSort}>
                  Package
                </SortTh>
                <th>Current</th>
                <th>Latest</th>
                <SortTh col="gap" sortKey={sortKey} sortDir={sortDir} onSort={handleSort}>
                  Gap
                </SortTh>
                <SortTh col="behind" sortKey={sortKey} sortDir={sortDir} onSort={handleSort}>
                  Behind
                </SortTh>
                <SortTh col="cves" sortKey={sortKey} sortDir={sortDir} onSort={handleSort}>
                  CVEs
                </SortTh>
                <SortTh col="score" sortKey={sortKey} sortDir={sortDir} onSort={handleSort}>
                  Score
                </SortTh>
                <th>Flags</th>
                <th>Top Reason</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((d, i) => {
                const cveCount = d.vulnerabilities?.length ?? 0
                const cveSeverity = cveCount > 0 ? d.vulnerabilities[0].severity : null
                const isBlocked = d.blocked_by?.length > 0
                const isCascade = !!d.cascade_group
                const color = isCascade ? cascadeColor(d.cascade_group) : null

                return (
                  <tr key={`${d.name}-${d.repo_source}-${i}`}>
                    <td style={{ width: 3, padding: 0, background: color ?? 'transparent', borderBottom: 'none' }} />
                    <td className="text-muted">{i + 1}</td>
                    {showRepo && (
                      <td className="monospace text-muted" style={{ whiteSpace: 'nowrap' }}>
                        {d.repo_source || '—'}
                      </td>
                    )}
                    <td>
                      <strong>{d.name}</strong>
                      <br />
                      <span className="text-muted monospace">{d.ecosystem}</span>
                    </td>
                    <td className="monospace">{d.current_version || '—'}</td>
                    <td className="monospace">{d.latest_version || '—'}</td>
                    <td>{d.severity_gap || (d.latest_version ? '—' : 'n/a')}</td>
                    <td>{d.versions_behind ?? '—'}</td>
                    <td>
                      {cveCount > 0
                        ? <span className="text-red">{cveCount} ({cveSeverity})</span>
                        : '—'
                      }
                    </td>
                    <td><RiskBadge score={d.risk_score} /></td>
                    <td>
                      {isBlocked ? (
                        <span className="badge badge-high">BLOCKED</span>
                      ) : isCascade ? (
                        <span className="badge" style={{ background: `${color}22`, color, border: `1px solid ${color}55` }}>
                          CASCADE
                        </span>
                      ) : '—'}
                    </td>
                    <td className="text-muted">{d.reasons?.[0] ?? '—'}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
