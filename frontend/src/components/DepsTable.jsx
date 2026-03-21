import React from 'react'
import RiskBadge from './RiskBadge.jsx'

export default function DepsTable({ deps }) {
  if (!deps || deps.length === 0) {
    return <p className="empty">No dependencies recorded for this scan.</p>
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>#</th>
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
          {deps.map((d, i) => {
            const cveCount = d.vulnerabilities?.length ?? 0
            const severity = cveCount > 0 ? d.vulnerabilities[0].severity : null
            const flags = d.blocked_by?.length > 0
              ? 'BLOCKED'
              : d.cascade_group
                ? 'CASCADE'
                : ''
            const flagClass = flags === 'BLOCKED' ? 'text-red' : flags === 'CASCADE' ? 'text-yellow' : ''
            return (
              <tr key={`${d.name}-${i}`}>
                <td className="text-muted">{i + 1}</td>
                <td><strong>{d.name}</strong><br /><span className="text-muted monospace">{d.ecosystem}</span></td>
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
                <td className={flagClass}>{flags || '—'}</td>
                <td className="text-muted">{d.reasons?.[0] ?? '—'}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}
