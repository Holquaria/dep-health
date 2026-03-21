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
    // Poll while running.
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

  return (
    <div>
      <Link to="/" className="back">← All scans</Link>

      <div className="flex items-center gap-8 mb-8">
        <h1 className="page-title" style={{ marginBottom: 0 }}>Scan #{run.id}</h1>
        <StatusBadge status={run.status} />
      </div>

      <div className="card">
        <div className="flex gap-8" style={{ flexWrap: 'wrap' }}>
          <div>
            <div className="text-muted" style={{ fontSize: 11 }}>DIRECTORY</div>
            <div className="monospace">{run.dir}</div>
          </div>
          {run.repo_url && (
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
              <div>
                {((new Date(run.finished_at) - new Date(run.started_at)) / 1000).toFixed(1)}s
              </div>
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

      {run.status === 'running' && (
        <p className="text-muted">Scan in progress — refreshing automatically…</p>
      )}

      {deps && deps.length > 0 && (
        <>
          <div className="card-title mt-16">{deps.length} dependencies</div>
          <DepsTable deps={deps} />
          <CascadePanel deps={deps} />
          <BlockedPanel deps={deps} />

          <MigrationHints deps={deps} />
        </>
      )}
    </div>
  )
}

function MigrationHints({ deps }) {
  const top = deps.filter(d => d.migration_steps?.length > 0).slice(0, 3)
  if (top.length === 0) return null

  return (
    <div className="panel mt-16">
      <div className="panel-title">Migration Hints — top risk</div>
      {top.map(d => (
        <div key={d.name} className="mb-8">
          <div className="flex gap-8 items-center mb-8">
            <strong>{d.name}</strong>
            <span className="monospace text-muted">{d.current_version} → {d.latest_version}</span>
          </div>
          {d.summary && <p className="text-muted" style={{ marginBottom: 6 }}>{d.summary}</p>}
          <ol style={{ paddingLeft: 20 }}>
            {d.migration_steps.map((step, i) => (
              <li key={i} style={{ marginBottom: 2 }}>{step}</li>
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
