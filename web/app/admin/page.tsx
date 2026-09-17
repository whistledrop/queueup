'use client'

// The operator's screen, for Logan, on his phone.
//
// It asks for the admin token rather than using a sign-in, because it is not an
// account feature: it is the operator's window onto the whole relay. The token
// is kept in this browser only and sent straight to the relay's admin endpoint.
//
// During the beta it answers three questions, in this order: are joins working
// (outcomes), who had a bad time (failures), and what are people saying
// (feedback and problem reports). Then the people themselves.

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

type Outcome = { state: string; reason: string; count: number }

type Failure = {
  job_id: string
  email: string
  server_name: string
  reason: string
  message: string
  attempts: number
  at: string
}

type Feedback = {
  id: string
  email: string
  device_name?: string
  kind: 'feedback' | 'report'
  message: string
  agent_version?: string
  body_bytes: number
  created_at: string
}

type Status = {
  connected_agents: number
  accounts: Account[]
  devices: Device[]
  recent_jobs: Job[]
  outcomes_24h: Outcome[]
  outcomes_7d: Outcome[]
  failures_7d: Failure[]
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

// Outcomes grouped the way a person thinks about them.
function summarise(list: Outcome[] = []) {
  let joined = 0, cancelled = 0, failed = 0, running = 0
  const failReasons: Record<string, number> = {}
  for (const o of list) {
    if (o.state === 'done') {
      if (['cancelled', 'player_closed', 'player_left'].includes(o.reason)) cancelled += o.count
      else joined += o.count
    } else if (o.state === 'failed') {
      failed += o.count
      const key = o.reason || 'unknown'
      failReasons[key] = (failReasons[key] ?? 0) + o.count
    } else {
      running += o.count
    }
  }
  const total = joined + cancelled + failed + running
  return { total, joined, cancelled, failed, running, failReasons }
}

function kb(n: number): string {
  return n < 1024 ? `${n} B` : `${Math.round(n / 1024)} KB`
}

export default function AdminPage() {
  const [token, setToken] = useState('')
  const [status, setStatus] = useState<Status | null>(null)
  const [feedback, setFeedback] = useState<Feedback[]>([])
  const [openReport, setOpenReport] = useState<{ id: string; body: string } | null>(null)
  const [notice, setNotice] = useState('')
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

  const call = useCallback(
    (path: string, init?: RequestInit) =>
      fetch('/api/relay' + path, {
        ...init,
        headers: {
          'x-admin-token': token,
          ...(init?.body ? { 'content-type': 'application/json' } : {}),
        },
      }),
    [token],
  )

  const load = useCallback(async () => {
    if (!token) return
    setLoading(true)
    try {
      const [s, f] = await Promise.all([call('/admin/status'), call('/admin/feedback')])
      if (!s.ok) {
        setError(s.status === 401 ? 'That admin token was refused.' : `Relay said ${s.status}.`)
        setStatus(null)
        return
      }
      setStatus(await s.json())
      if (f.ok) setFeedback((await f.json()).feedback ?? [])
      setError('')
      try { localStorage.setItem(TOKEN_KEY, token) } catch {}
    } catch {
      setError("Couldn't reach the relay.")
    } finally {
      setLoading(false)
    }
  }, [call, token])

  useEffect(() => {
    if (!token || !status) return
    const t = setInterval(load, 20000)
    return () => clearInterval(t)
  }, [load, token, status])

  async function viewReport(id: string) {
    if (openReport?.id === id) {
      setOpenReport(null)
      return
    }
    const res = await call(`/admin/feedback/${id}`)
    setOpenReport({ id, body: res.ok ? await res.text() : `Could not load it (${res.status}).` })
  }

  async function deleteFeedback(id: string) {
    if (!confirm('Delete this for good?')) return
    const res = await call(`/admin/feedback/${id}`, { method: 'DELETE' })
    if (res.ok) {
      setFeedback((list) => list.filter((f) => f.id !== id))
      if (openReport?.id === id) setOpenReport(null)
    } else {
      setError(`Delete failed (${res.status}).`)
    }
  }

  async function tempPassword(a: Account) {
    if (!confirm(`Give ${a.email} a new temporary password? Their old one stops working and they are signed out everywhere.`)) return
    const res = await call(`/admin/accounts/${a.id}/temp-password`, { method: 'POST' })
    const body = await res.json().catch(() => ({}))
    if (!res.ok) {
      setError(body.error ?? `Failed (${res.status}).`)
      return
    }
    setNotice(`Temporary password for ${a.email}: ${body.password}  (send it to them privately; it is not shown again)`)
  }

  async function erase(a: Account) {
    const typed = prompt(
      `This deletes ${a.email} and EVERYTHING about them: PC, joins, feedback, reports. It cannot be undone.\n\nType their email address to confirm:`,
    )
    if (typed === null) return
    const res = await call(`/admin/accounts/${a.id}/erase`, {
      method: 'POST',
      body: JSON.stringify({ confirm_email: typed }),
    })
    const body = await res.json().catch(() => ({}))
    if (!res.ok) {
      setError(body.error ?? `Failed (${res.status}).`)
      return
    }
    setNotice(`${a.email} has been erased.`)
    load()
  }

  const day = summarise(status?.outcomes_24h)
  const week = summarise(status?.outcomes_7d)

  return (
    <div className="shell">
      <header className="top">
        <span className="brand">Queue<span>Up</span></span>
        <span className="muted small">admin</span>
      </header>

      {error && <div className="error">{error}</div>}
      {notice && (
        <div className="notice" style={{ wordBreak: 'break-word' }}>
          {notice}{' '}
          <button className="quiet" style={{ minHeight: 0, padding: '2px 8px' }} onClick={() => setNotice('')}>
            dismiss
          </button>
        </div>
      )}

      {!status && (
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
            <button className="primary btn-wide" onClick={load} disabled={loading || !token}>
              {loading ? 'Loading' : 'Show me'}
            </button>
          </div>
        </div>
      )}

      {status && (
        <>
          <div className="card">
            <h2>Right now</h2>
            <div className="row">
              <Stat n={status.accounts?.length ?? 0} label="accounts" />
              <Stat n={status.devices?.length ?? 0} label="linked PCs" />
              <Stat n={status.connected_agents} label="online now" />
            </div>
          </div>

          <div className="card">
            <h2>How joins went</h2>
            <OutcomeRow title="Last 24 hours" s={day} />
            <OutcomeRow title="Last 7 days" s={week} />
            {Object.keys(week.failReasons).length > 0 && (
              <p className="muted small" style={{ marginBottom: 0 }}>
                Failures this week by reason:{' '}
                {Object.entries(week.failReasons)
                  .sort((a, b) => b[1] - a[1])
                  .map(([r, n]) => `${r} ${n}`)
                  .join(' · ')}
              </p>
            )}
          </div>

          <div className="card">
            <h2>Failed joins, last 7 days</h2>
            {(status.failures_7d ?? []).map((f) => (
              <div className="server" key={f.job_id}>
                <div className="name">{f.server_name}</div>
                <div className="muted small">
                  {f.email} · {when(f.at)} · {f.attempts} attempt{f.attempts === 1 ? '' : 's'}
                </div>
                <div className="small">
                  <span className="pill bad">{f.reason || 'unknown'}</span> {f.message}
                </div>
              </div>
            ))}
            {(status.failures_7d ?? []).length === 0 && <div className="muted">None. Good.</div>}
          </div>

          <div className="card">
            <h2>Feedback and problem reports</h2>
            {feedback.map((f) => (
              <div className="server" key={f.id}>
                <div className="row">
                  <div style={{ minWidth: 0 }}>
                    <div className="muted small">
                      <span className={`pill ${f.kind === 'report' ? 'warn' : ''}`}>
                        {f.kind === 'report' ? 'report' : 'feedback'}
                      </span>{' '}
                      {f.email} · {when(f.created_at)}
                      {f.kind === 'report' && ` · ${f.device_name || 'PC'} ${f.agent_version ?? ''} · ${kb(f.body_bytes)}`}
                    </div>
                    {f.message && <div style={{ whiteSpace: 'pre-wrap', marginTop: 4 }}>{f.message}</div>}
                  </div>
                </div>
                <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                  {f.kind === 'report' && (
                    <button style={{ minHeight: 34, padding: '4px 10px' }} onClick={() => viewReport(f.id)}>
                      {openReport?.id === f.id ? 'Hide report' : 'Read report'}
                    </button>
                  )}
                  <button className="danger" style={{ minHeight: 34, padding: '4px 10px' }} onClick={() => deleteFeedback(f.id)}>
                    Delete
                  </button>
                </div>
                {openReport?.id === f.id && <pre className="reportBody">{openReport.body}</pre>}
              </div>
            ))}
            {feedback.length === 0 && <div className="muted">Nothing yet.</div>}
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
                <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                  <button style={{ minHeight: 34, padding: '4px 10px' }} onClick={() => tempPassword(a)}>
                    Temporary password
                  </button>
                  <button className="danger" style={{ minHeight: 34, padding: '4px 10px' }} onClick={() => erase(a)}>
                    Erase
                  </button>
                </div>
              </div>
            ))}
            {(status.accounts ?? []).length === 0 && <div className="muted">Nobody has signed up yet.</div>}
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
    </div>
  )
}

function Stat({ n, label }: { n: number; label: string }) {
  return (
    <div>
      <div style={{ fontSize: 30, fontWeight: 800 }}>{n}</div>
      <div className="muted small">{label}</div>
    </div>
  )
}

function OutcomeRow({ title, s }: { title: string; s: ReturnType<typeof summarise> }) {
  const pct = (n: number) => (s.total ? ` (${Math.round((n / s.total) * 100)}%)` : '')
  return (
    <div className="server">
      <div className="name">
        {title}: {s.total} join{s.total === 1 ? '' : 's'}
      </div>
      <div className="small">
        <span className="pill good">got in {s.joined}{pct(s.joined)}</span>{' '}
        <span className="pill">cancelled {s.cancelled}</span>{' '}
        <span className="pill bad">failed {s.failed}{pct(s.failed)}</span>
        {s.running > 0 && <> <span className="pill warn">still running {s.running}</span></>}
      </div>
    </div>
  )
}
