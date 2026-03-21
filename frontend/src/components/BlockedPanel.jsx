import React from 'react'

export default function BlockedPanel({ deps }) {
  const blocked = deps.filter(d => d.blocked_by?.length > 0)
  if (blocked.length === 0) return null

  return (
    <div className="panel mt-16">
      <div className="panel-title">Blocked Dependencies — no safe upgrade path</div>
      {blocked.map(d => (
        <div key={d.name} className="mb-8">
          <strong className="text-red">{d.name}</strong>
          {' '}is blocked by:{' '}
          {d.blocked_by.map(peer => {
            const constraint = d.peer_constraints?.[peer] ?? ''
            return (
              <span key={peer} className="text-muted">
                {peer}{constraint ? ` (requires ${constraint})` : ''}
              </span>
            )
          }).reduce((acc, el, i) => i === 0 ? [el] : [...acc, ', ', el], [])}
        </div>
      ))}
    </div>
  )
}
