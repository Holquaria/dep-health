import React from 'react'

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
      {keys.map(group => (
        <div key={group} className="mb-8">
          <div className="text-yellow mb-8">{group.replaceAll('+', ' + ')}</div>
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
                  <td>{d.name}</td>
                  <td className="monospace">{d.current_version}</td>
                  <td className="monospace">{d.latest_version}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </div>
  )
}
