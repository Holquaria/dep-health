import React from 'react'

export default function RiskBadge({ score }) {
  const s = Number(score)
  let cls = 'badge-low', label = 'Low'
  if (s >= 70) { cls = 'badge-high';   label = 'High' }
  else if (s >= 40) { cls = 'badge-medium'; label = 'Med' }
  return <span className={`badge ${cls}`}>{s.toFixed(1)} {label}</span>
}
