import React from 'react'
import RiskBadge from './RiskBadge.jsx'
import { cascadeColor } from './cascadeColor.js'

export default function DepsTable({ deps, showRepo = false, repoFilter = null }) {
  if (!deps || deps.length === 0) {
    return <p className="empty">No dependencies recorded for this scan.</p>
  }

  const visible = repoFilter
    ? deps.filter(d => d.repo_source === repoFilter)
    : deps

  if (visible.length === 0) {
    return <p className="empty">No dependencies match the selected repo.</p>
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th style={{ width: 3, padding: 0 }} />
            <th>#</th>
            {showRepo && <th>Repo</th>}
            <th>Package</th>
            <th>Current</th>
            <th>Latest</th>
            <th>Gap</th>
            <th>Behind</th>
            <th>CVEs</th>
            <th>Score</th>
            <th>Flags</th>
            <th>Top Reason</th>
          </tr>
        </thead>
        <tbody>
          {visible.map((d, i) => {
            const cveCount = d.vulnerabilities?.length ?? 0
            const severity = cveCount > 0 ? d.vulnerabilities[0].severity : null
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
                    ? <span className="text-red">{cveCount} ({severity})</span>
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
  )
}
