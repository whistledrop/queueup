'use client'

// The customer list, for Logan, on his phone.
//
// It asks for the admin token rather than using a sign-in, because it is not an
// account feature: it is the operator's window onto the whole relay. The token
// is kept in this browser only and sent straight to the relay's admin endpoint.

import { useCallback, useEffect, useState } from 'react'

type Account = {
  id: string
  email: string
  created_at: string
  devices: number
  jobs: number
  last_job_at?: string
  last_seen_at?: string
  subscription: string
}

type Device = {
  id: string
  name: string
  online: boolean
  agent_version: string
  simulator: boolean
  last_seen_at: string
}

type Job = {
  id: string
  server_name: string
  server_addr: string
  state: string
  detail: string
  updated_at: string
}

type Status = {
  connected_agents: number
  accounts: Account[]
  devices: Device[]
  recent_jobs: Job[]
}

const TOKEN_KEY = 'queueup_admin_token'

function when(iso?: string): string {
  if (!iso || iso.startsWith('0001')) return 'never'
  const d = new Date(iso)
  const mins = Math.round((Date.now() - d.getTime()) / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  if (mins < 60 * 24) return `${Math.round(mins / 60)}h ago`
  return d.toLocaleDateString()
}

export default function AdminPage() {
  const [token, setToken] = useState('')
  const [status, setStatus] = useState<Status | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    try {
      const saved = localStorage.getItem(TOKEN_KEY)
      if (saved) setToken(saved)
    } catch {
      // private browsing; the token just has to be typed each time
    }
  }, [])

  const load = useCallback(async (t: string) => {
    if (!t) return
    setLoading(true)
    try {
      const res = await fetch('/api/relay/admin/status', {
        headers: { 'x-admin-token': t },
      })
      if (!res.ok) {
        setError(res.status === 401 ? 'That admin token was refused.' : `Relay said ${res.status}.`)
        setStatus(null)
        return
      }
      setStatus(await res.json())
      setError('')
      try { localStorage.setItem(TOKEN_KEY, t) } catch {}
    } catch {
      setError("Couldn't reach the relay.")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!token) return
    load(token)
    const t = setInterval(() => load(token), 15000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status === null ? token : ''])

  return (
    <>
      <header className="top">
        <span className="brand">Queue<span>Up</span></span>
        <span className="muted small">admin</span>
      </header>

      {error && <div className="error">{error}</div>}

      <div className="card">
        <h2>Admin token</h2>
        <div className="stack">
          <input
            type="password"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            placeholder="QUEUEUP_ADMIN_TOKEN"
            aria-label="Admin token"
          />
          <button className="primary btn-wide" onClick={() => load(token)} disabled={loading || !token}>
            {loading ? 'Loading' : 'Show me'}
          </button>
        </div>
      </div>

      {status && (
        <>
          <div className="card">
            <h2>Right now</h2>
            <div className="row">
              <div>
                <div style={{ fontSize: 30, fontWeight: 800 }}>{status.accounts?.length ?? 0}</div>
                <div className="muted small">accounts</div>
              </div>
              <div>
                <div style={{ fontSize: 30, fontWeight: 800 }}>{status.devices?.length ?? 0}</div>
                <div className="muted small">linked PCs</div>
              </div>
              <div>
                <div style={{ fontSize: 30, fontWeight: 800 }}>{status.connected_agents}</div>
                <div className="muted small">online now</div>
              </div>
            </div>
          </div>

          <div className="card">
            <h2>Accounts</h2>
            {(status.accounts ?? []).map((a) => (
              <div className="server" key={a.id}>
                <div className="row">
                  <div style={{ minWidth: 0 }}>
                    <div className="name" style={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>
                      {a.email}
                    </div>
                    <div className="muted small">
                      joined {when(a.created_at)} · {a.devices} PC{a.devices === 1 ? '' : 's'} ·{' '}
                      {a.jobs} join{a.jobs === 1 ? '' : 's'}
                      {a.jobs > 0 && `, last ${when(a.last_job_at)}`}
                    </div>
                  </div>
                  {a.subscription === 'active' && <span className="pill good">paying</span>}
                </div>
              </div>
            ))}
            {(status.accounts ?? []).length === 0 && (
              <div className="muted">Nobody has signed up yet.</div>
            )}
          </div>

          <div className="card">
            <h2>PCs</h2>
            {(status.devices ?? []).map((d) => (
              <div className="server" key={d.id}>
                <div className="row">
                  <div style={{ minWidth: 0 }}>
                    <div className="name">
                      <span className={`dot ${d.online ? 'on' : 'off'}`} />
                      {d.name}
                    </div>
                    <div className="muted small">
                      {d.agent_version || 'unknown version'} · seen {when(d.last_seen_at)}
                    </div>
                  </div>
                  {d.simulator && <span className="pill warn">sim</span>}
                </div>
              </div>
            ))}
            {(status.devices ?? []).length === 0 && <div className="muted">No PCs linked yet.</div>}
          </div>

          <div className="card">
            <h2>Recent joins</h2>
            {(status.recent_jobs ?? []).map((j) => (
              <div className="server" key={j.id}>
                <div className="row">
                  <div style={{ minWidth: 0 }}>
                    <div className="name">{j.server_name || j.server_addr}</div>
                    <div className="muted small">{j.detail}</div>
                  </div>
                  <span className="pill">{j.state}</span>
                </div>
              </div>
            ))}
            {(status.recent_jobs ?? []).length === 0 && <div className="muted">No joins yet.</div>}
          </div>
        </>
      )}
    </>
  )
}
