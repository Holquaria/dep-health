import React, { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { listScans, triggerScan } from '../api.js'
import StatusBadge from '../components/StatusBadge.jsx'

export default function ScanList() {
  const [scans, setScans] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [dir, setDir] = useState('')
  const [repoUrl, setRepoUrl] = useState('')
  const [triggering, setTriggering] = useState(false)

  const load = useCallback(async () => {
    try {
      const data = await listScans()
      setScans(data ?? [])
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    // Poll every 3s while any scan is running.
    const id = setInterval(() => {
      setScans(prev => {
        if (prev.some(s => s.status === 'running' || s.status === 'pending')) {
          load()
        }
        return prev
      })
    }, 3000)
    return () => clearInterval(id)
  }, [load])

  async function handleTrigger(e) {
    e.preventDefault()
    if (!dir.trim()) return
    setTriggering(true)
    try {
      await triggerScan(dir.trim(), repoUrl.trim())
      setDir('')
      setRepoUrl('')
      await load()
    } catch (e) {
      setError(e.message)
    } finally {
      setTriggering(false)
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-8">
        <h1 className="page-title" style={{ marginBottom: 0 }}>Scans</h1>
      </div>

      {/* Trigger form */}
      <div className="card">
        <div className="card-title">New Scan</div>
        <form onSubmit={handleTrigger}>
          <div className="form-row">
            <div className="form-field">
              <label className="form-label">Directory (absolute path)</label>
              <input
                className="input"
                value={dir}
                onChange={e => setDir(e.target.value)}
                placeholder="/path/to/repo"
                required
              />
            </div>
            <div className="form-field">
              <label className="form-label">Repo URL (optional)</label>
              <input
                className="input"
                value={repoUrl}
                onChange={e => setRepoUrl(e.target.value)}
                placeholder="https://github.com/org/repo"
              />
            </div>
            <button className="btn btn-primary" type="submit" disabled={triggering}>
              {triggering ? 'Starting…' : 'Run Scan'}
            </button>
          </div>
        </form>
      </div>

      {error && <p className="text-red">{error}</p>}

      {loading ? (
        <p className="text-muted">Loading…</p>
      ) : scans.length === 0 ? (
        <div className="empty">No scans yet. Run your first scan above.</div>
      ) : (
        <div className="table-wrap">
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Directory</th>
                <th>Status</th>
                <th>Started</th>
                <th>Duration</th>
              </tr>
            </thead>
            <tbody>
              {scans.map(s => {
                const started = new Date(s.started_at)
                const finished = s.finished_at ? new Date(s.finished_at) : null
                const duration = finished
                  ? `${((finished - started) / 1000).toFixed(1)}s`
                  : s.status === 'running' ? '…' : '—'
                return (
                  <tr key={s.id}>
                    <td><Link to={`/scans/${s.id}`}>#{s.id}</Link></td>
                    <td className="monospace">{s.dir}</td>
                    <td><StatusBadge status={s.status} /></td>
                    <td className="text-muted">{started.toLocaleString()}</td>
                    <td className="text-muted">{duration}</td>
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
