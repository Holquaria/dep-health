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

export async function triggerScan(dir = '', gitUrl = '', repoUrl = '') {
  const body = { repo_url: repoUrl || gitUrl || undefined }
  if (gitUrl) body.git_url = gitUrl
  else body.dir = dir
  const res = await fetch(`${BASE}/scans`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) throw new Error(await res.text())
  return res.json()
}
