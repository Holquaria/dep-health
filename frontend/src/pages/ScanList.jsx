import React, { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { listScans, triggerScan } from '../api.js'
import StatusBadge from '../components/StatusBadge.jsx'

export default function ScanList() {
  const [scans, setScans] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // form state
  const [mode, setMode] = useState('local') // 'local' | 'remote'
  const [dir, setDir] = useState('')
  const [gitUrl, setGitUrl] = useState('')
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
    setTriggering(true)
    setError(null)
    try {
      await triggerScan(
        mode === 'local' ? dir.trim() : '',
        mode === 'remote' ? gitUrl.trim() : '',
        repoUrl.trim(),
      )
      setDir('')
      setGitUrl('')
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
        <div className="flex items-center gap-8 mb-8">
          <div className="card-title" style={{ marginBottom: 0 }}>New Scan</div>
          <div className="flex gap-8" style={{ marginLeft: 'auto' }}>
            <button
              type="button"
              className={`btn ${mode === 'local' ? 'btn-primary' : ''}`}
              onClick={() => setMode('local')}
            >
              Local path
            </button>
            <button
              type="button"
              className={`btn ${mode === 'remote' ? 'btn-primary' : ''}`}
              onClick={() => setMode('remote')}
            >
              Git URL
            </button>
          </div>
        </div>

        <form onSubmit={handleTrigger}>
          <div className="form-row">
            {mode === 'local' ? (
              <div className="form-field">
                <label className="form-label">Directory (absolute path)</label>
                <input
                  className="input"
                  value={dir}
                  onChange={e => setDir(e.target.value)}
                  placeholder="/Users/you/your-project"
                  required
                />
              </div>
            ) : (
              <div className="form-field">
                <label className="form-label">Git URL</label>
                <input
                  className="input"
                  value={gitUrl}
                  onChange={e => setGitUrl(e.target.value)}
                  placeholder="https://github.com/org/repo"
                  required
                />
              </div>
            )}
            <div className="form-field" style={{ maxWidth: 280 }}>
              <label className="form-label">Repo URL override (optional)</label>
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
          {mode === 'remote' && (
            <p className="text-muted" style={{ fontSize: 12, marginTop: -8 }}>
              The server will run <code>git clone --depth 1</code>. For private repos set <code>GITHUB_TOKEN</code> on the server.
            </p>
          )}
        </form>
      </div>

      {error && <p className="text-red" style={{ marginBottom: 12 }}>{error}</p>}

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
                <th>Target</th>
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
                    <td className="monospace">
                      {s.repo_url
                        ? <a href={s.repo_url} target="_blank" rel="noreferrer">{s.dir}</a>
                        : s.dir
                      }
                    </td>
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
