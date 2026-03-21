import React, { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { listScans, triggerScan, triggerMultiScan } from '../api.js'
import StatusBadge from '../components/StatusBadge.jsx'

export default function ScanList() {
  const [scans, setScans] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  // form state — mode: 'local' | 'remote' | 'multi'
  const [mode, setMode] = useState('local')
  const [dir, setDir] = useState('')
  const [gitUrl, setGitUrl] = useState('')
  const [repoUrl, setRepoUrl] = useState('')
  // multi mode: list of target strings
  const [multiTargets, setMultiTargets] = useState(['', ''])
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
      if (mode === 'multi') {
        const targets = multiTargets.map(t => t.trim()).filter(Boolean)
        if (targets.length < 2) throw new Error('Provide at least 2 targets')
        await triggerMultiScan(targets)
        setMultiTargets(['', ''])
      } else {
        await triggerScan(
          mode === 'local' ? dir.trim() : '',
          mode === 'remote' ? gitUrl.trim() : '',
          repoUrl.trim(),
        )
        setDir('')
        setGitUrl('')
        setRepoUrl('')
      }
      await load()
    } catch (e) {
      setError(e.message)
    } finally {
      setTriggering(false)
    }
  }

  function updateMultiTarget(index, value) {
    // Auto-detect: URL in any field stays as-is (server handles it)
    setMultiTargets(prev => prev.map((t, i) => i === index ? value : t))
  }

  function addMultiTarget() {
    setMultiTargets(prev => [...prev, ''])
  }

  function removeMultiTarget(index) {
    setMultiTargets(prev => prev.filter((_, i) => i !== index))
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
            <button type="button" className={`btn ${mode === 'local' ? 'btn-primary' : ''}`}
              onClick={() => setMode('local')}>Local path</button>
            <button type="button" className={`btn ${mode === 'remote' ? 'btn-primary' : ''}`}
              onClick={() => setMode('remote')}>Git URL</button>
            <button type="button" className={`btn ${mode === 'multi' ? 'btn-primary' : ''}`}
              onClick={() => setMode('multi')}>Multi-repo</button>
          </div>
        </div>

        <form onSubmit={handleTrigger}>
          {mode === 'multi' ? (
            <div>
              <div className="text-muted" style={{ fontSize: 12, marginBottom: 8 }}>
                Enter local paths or git URLs — one per line. At least 2 required.
              </div>
              {multiTargets.map((target, i) => (
                <div key={i} className="flex gap-8 mb-8" style={{ alignItems: 'center' }}>
                  <input
                    className="input"
                    value={target}
                    onChange={e => updateMultiTarget(i, e.target.value)}
                    placeholder={i === 0 ? '/path/to/repo or https://github.com/org/repo' : 'another repo…'}
                    required={i < 2}
                    style={{ flex: 1 }}
                  />
                  {multiTargets.length > 2 && (
                    <button type="button" className="btn" onClick={() => removeMultiTarget(i)}
                      style={{ padding: '4px 10px' }}>✕</button>
                  )}
                </div>
              ))}
              <div className="flex gap-8" style={{ marginTop: 4 }}>
                <button type="button" className="btn" onClick={addMultiTarget}>+ Add another</button>
                <button className="btn btn-primary" type="submit" disabled={triggering}
                  style={{ marginLeft: 'auto' }}>
                  {triggering ? 'Starting…' : 'Run Multi-Scan'}
                </button>
              </div>
            </div>
          ) : (
            <div className="form-row">
              {mode === 'local' ? (
                <div className="form-field">
                  <label className="form-label">Directory (absolute path)</label>
                  <input
                    className="input"
                    value={dir}
                    onChange={e => {
                      const v = e.target.value
                      if (v.startsWith('http://') || v.startsWith('https://') || v.startsWith('git@') || v.startsWith('git://')) {
                        setMode('remote')
                        setGitUrl(v)
                        setDir('')
                      } else {
                        setDir(v)
                      }
                    }}
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
          )}
          {mode === 'remote' && (
            <p className="text-muted" style={{ fontSize: 12, marginTop: 4 }}>
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
                const targetLabel = s.is_multi
                  ? `${s.dir} (${(s.targets ?? []).join(', ')})`
                  : s.dir
                return (
                  <tr key={s.id}>
                    <td><Link to={`/scans/${s.id}`}>#{s.id}</Link></td>
                    <td className="monospace" style={{ maxWidth: 400, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {s.repo_url
                        ? <a href={s.repo_url} target="_blank" rel="noreferrer">{s.dir}</a>
                        : targetLabel
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
