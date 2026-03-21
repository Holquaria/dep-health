import React from 'react'

export default function StatusBadge({ status }) {
  const map = { done: 'badge-done', running: 'badge-running', failed: 'badge-failed', pending: 'badge-pending' }
  return <span className={`badge ${map[status] || 'badge-pending'}`}>{status}</span>
}
