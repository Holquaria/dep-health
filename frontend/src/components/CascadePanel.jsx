import React from 'react'
import { cascadeColor } from './cascadeColor.js'

export default function CascadePanel({ deps }) {
  const groups = {}
  for (const d of deps) {
    if (d.cascade_group) {
      groups[d.cascade_group] = groups[d.cascade_group] || []
      groups[d.cascade_group].push(d)
    }
  }

  const keys = Object.keys(groups)
  if (keys.length === 0) return null

  return (
    <div className="panel mt-16">
      <div className="panel-title">Cascade Groups — must upgrade together</div>
      {keys.map(group => {
        const color = cascadeColor(group)
        return (
          <div key={group} className="mb-8" style={{ borderLeft: `3px solid ${color}`, paddingLeft: 12 }}>
            <div style={{ color, fontWeight: 600, marginBottom: 8 }}>
              {group.replaceAll('+', ' + ')}
            </div>
            <table>
              <thead>
                <tr>
                  <th>Package</th>
                  <th>Current</th>
                  <th>Latest</th>
                </tr>
              </thead>
              <tbody>
                {groups[group].map(d => (
                  <tr key={d.name}>
                    <td>
                      <span
                        style={{
                          display: 'inline-block',
                          width: 8,
                          height: 8,
                          borderRadius: '50%',
                          background: color,
                          marginRight: 6,
                          verticalAlign: 'middle',
                        }}
                      />
                      {d.name}
                    </td>
                    <td className="monospace">{d.current_version}</td>
                    <td className="monospace">{d.latest_version}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      })}
    </div>
  )
}
