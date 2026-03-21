const BASE = '/api'

export async function listScans() {
  const res = await fetch(`${BASE}/scans`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function getScan(id) {
  const res = await fetch(`${BASE}/scans/${id}`)
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}

export async function triggerScan(dir, repoUrl = '') {
  const res = await fetch(`${BASE}/scans`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ dir, repo_url: repoUrl }),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
