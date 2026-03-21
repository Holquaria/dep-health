import React, { useState, useEffect, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { getScan } from '../api.js'
import StatusBadge from '../components/StatusBadge.jsx'
import DepsTable from '../components/DepsTable.jsx'
import CascadePanel from '../components/CascadePanel.jsx'
import BlockedPanel from '../components/BlockedPanel.jsx'

export default function ScanDetail() {
  const { id } = useParams()
  const [data, setData] = useState(null)
  const [error, setError] = useState(null)
  const [repoFilter, setRepoFilter] = useState(null) // null = show all

  const load = useCallback(async () => {
    try {
      const d = await getScan(id)
      setData(d)
    } catch (e) {
      setError(e.message)
    }
  }, [id])

  useEffect(() => {
    load()
    let interval
    interval = setInterval(() => {
      setData(prev => {
        if (prev?.run?.status === 'running' || prev?.run?.status === 'pending') {
          load()
        }
        return prev
      })
    }, 2000)
    return () => clearInterval(interval)
  }, [load])

  if (error) return <p className="text-red">{error}</p>
  if (!data) return <p className="text-muted">Loading…</p>

  const { run, deps } = data
  const isMulti = run.is_multi

  // Unique repo labels from deps (for filter).
  const repos = isMulti
    ? [...new Set((deps ?? []).map(d => d.repo_source).filter(Boolean))].sort()
    : []

  // Stats derived from deps for multi scans.
  const stats = isMulti ? computeStats(deps ?? []) : null

  const visibleDeps = repoFilter
    ? (deps ?? []).filter(d => d.repo_source === repoFilter)
    : (deps ?? [])

  return (
    <div>
      <Link to="/" className="back">← All scans</Link>

      <div className="flex items-center gap-8 mb-8">
        <h1 className="page-title" style={{ marginBottom: 0 }}>Scan #{run.id}</h1>
        <StatusBadge status={run.status} />
        {isMulti && (
          <span className="badge" style={{ background: '#6366f133', color: '#6366f1', border: '1px solid #6366f155' }}>
            MULTI-REPO
          </span>
        )}
      </div>

      {/* Run metadata */}
      <div className="card">
        <div className="flex gap-8" style={{ flexWrap: 'wrap' }}>
          {isMulti ? (
            <div>
              <div className="text-muted" style={{ fontSize: 11 }}>REPOSITORIES</div>
              <div className="monospace">
                {(run.targets ?? [run.dir]).map((t, i) => (
                  <div key={i}>{t}</div>
                ))}
              </div>
            </div>
          ) : (
            <div>
              <div className="text-muted" style={{ fontSize: 11 }}>DIRECTORY</div>
              <div className="monospace">{run.dir}</div>
            </div>
          )}
          {run.repo_url && !isMulti && (
            <div>
              <div className="text-muted" style={{ fontSize: 11 }}>REPO</div>
              <div><a href={run.repo_url} target="_blank" rel="noreferrer">{run.repo_url}</a></div>
            </div>
          )}
          <div>
            <div className="text-muted" style={{ fontSize: 11 }}>STARTED</div>
            <div>{new Date(run.started_at).toLocaleString()}</div>
          </div>
          {run.finished_at && (
            <div>
              <div className="text-muted" style={{ fontSize: 11 }}>DURATION</div>
              <div>{((new Date(run.finished_at) - new Date(run.started_at)) / 1000).toFixed(1)}s</div>
            </div>
          )}
          {run.error && (
            <div>
              <div className="text-muted" style={{ fontSize: 11 }}>ERROR</div>
              <div className="text-red">{run.error}</div>
            </div>
          )}
        </div>
      </div>

      {/* Multi-repo aggregate stats card */}
      {isMulti && stats && (
        <div className="card" style={{ marginTop: 12 }}>
          <div className="card-title" style={{ marginBottom: 12 }}>Aggregate Summary</div>
          <div className="flex gap-8" style={{ flexWrap: 'wrap' }}>
            <Stat label="Repos" value={run.targets?.length ?? '—'} />
            <Stat label="Outdated deps" value={stats.totalOutdated} />
            <Stat label="Total CVEs" value={stats.totalCVEs} color={stats.totalCVEs > 0 ? '#ef4444' : undefined} />
            <Stat label="Cascade groups" value={stats.cascadeGroups} color={stats.cascadeGroups > 0 ? '#f59e0b' : undefined} />
            <Stat label="Blocked" value={stats.blockedDeps} color={stats.blockedDeps > 0 ? '#ef4444' : undefined} />
          </div>
        </div>
      )}

      {run.status === 'running' && (
        <p className="text-muted" style={{ marginTop: 12 }}>Scan in progress — refreshing automatically…</p>
      )}

      {deps && deps.length > 0 && (
        <>
          {/* Repo filter for multi scans */}
          {isMulti && repos.length > 0 && (
            <div className="flex gap-8 items-center mt-16" style={{ flexWrap: 'wrap' }}>
              <span className="text-muted" style={{ fontSize: 12 }}>Filter by repo:</span>
              <button
                className={`btn ${!repoFilter ? 'btn-primary' : ''}`}
                style={{ padding: '2px 10px', fontSize: 12 }}
                onClick={() => setRepoFilter(null)}
              >
                All ({deps.length})
              </button>
              {repos.map(repo => {
                const count = deps.filter(d => d.repo_source === repo).length
                return (
                  <button
                    key={repo}
                    className={`btn ${repoFilter === repo ? 'btn-primary' : ''}`}
                    style={{ padding: '2px 10px', fontSize: 12 }}
                    onClick={() => setRepoFilter(repoFilter === repo ? null : repo)}
                  >
                    {repo} ({count})
                  </button>
                )
              })}
            </div>
          )}

          <div className="card-title mt-16">
            {visibleDeps.length} {repoFilter ? `deps in ${repoFilter}` : 'dependencies'}
          </div>
          <DepsTable deps={deps} showRepo={isMulti} repoFilter={repoFilter} />
          <CascadePanel deps={visibleDeps} />
          <BlockedPanel deps={visibleDeps} />
          <MigrationHints deps={visibleDeps} />
        </>
      )}
    </div>
  )
}

function Stat({ label, value, color }) {
  return (
    <div style={{ minWidth: 100 }}>
      <div className="text-muted" style={{ fontSize: 11 }}>{label.toUpperCase()}</div>
      <div style={{ fontSize: 22, fontWeight: 700, color: color ?? 'inherit' }}>{value}</div>
    </div>
  )
}

function computeStats(deps) {
  const cascades = new Set()
  let totalCVEs = 0
  let blockedDeps = 0
  for (const d of deps) {
    totalCVEs += d.vulnerabilities?.length ?? 0
    if (d.blocked_by?.length > 0) blockedDeps++
    if (d.cascade_group) cascades.add(`${d.repo_source}:${d.cascade_group}`)
  }
  return {
    totalOutdated: deps.length,
    totalCVEs,
    cascadeGroups: cascades.size,
    blockedDeps,
  }
}

function MigrationHints({ deps }) {
  const top = deps.filter(d => d.migration_steps?.length > 0).slice(0, 3)
  if (top.length === 0) return null

  return (
    <div className="panel mt-16">
      <div className="panel-title">Migration Hints — top risk</div>
      {top.map((d, i) => (
        <div key={`${d.name}-${d.repo_source}-${i}`} className="mb-8">
          <div className="flex gap-8 items-center mb-8">
            <strong>{d.name}</strong>
            <span className="monospace text-muted">{d.current_version} → {d.latest_version}</span>
            {d.repo_source && (
              <span className="text-muted" style={{ fontSize: 11 }}>{d.repo_source}</span>
            )}
          </div>
          {d.summary && <p className="text-muted" style={{ marginBottom: 6 }}>{d.summary}</p>}
          <ol style={{ paddingLeft: 20 }}>
            {d.migration_steps.map((step, j) => (
              <li key={j} style={{ marginBottom: 2 }}>{step}</li>
            ))}
          </ol>
          {d.breaking_changes?.length > 0 && (
            <p className="text-yellow" style={{ marginTop: 4 }}>
              ⚠ {d.breaking_changes.join(' ')}
            </p>
          )}
        </div>
      ))}
    </div>
  )
}
