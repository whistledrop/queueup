'use client'

// Email, on the operator's screen: what the automatic emails have been doing,
// and campaigns Logan writes himself.
//
// A campaign is plain text, sent to one audience, a daily allowance at a time.
// The relay adds the unsubscribe link and the "unofficial" line to every one,
// so they are never written here. The order the screen walks through is the
// safe order: write it, send yourself a test, read it on your phone, then
// schedule it. Once scheduled the words are fixed.

import { useCallback, useEffect, useState } from 'react'

type Campaign = {
  id: string
  name: string
  subject: string
  body: string
  segment: string
  status: 'draft' | 'scheduled' | 'done' | 'cancelled'
  start_at: string
  daily_limit: number
  created_at: string
  sent: number
}

type Segment = { id: string; name: string; size: number }

type Overview = {
  campaigns: Campaign[]
  segments: Segment[]
  flows_30_days: Record<string, number>
  email_ready: boolean
}

// What each automatic email is, in the words Logan would use for it.
const FLOWS: { key: string; name: string; when: string }[] = [
  { key: 'first_join', name: 'First join worked', when: 'An hour after their first successful join' },
  { key: 'joins_low', name: '2 free joins left', when: 'At 8 of 10 free joins, once a month' },
  { key: 'joins_out', name: 'Out of free joins', when: 'At 10 of 10 free joins, once a month' },
  { key: 'wipe_week', name: 'Force wipe week', when: 'Monday to Wednesday before the first Thursday' },
]

// Starting points. Plain text, short, one link: the emails that get read.
const TEMPLATES: { label: string; segment: string; subject: string; body: string }[] = [
  {
    label: 'QueueUp is free now',
    segment: 'free',
    subject: 'QueueUp is free now: 10 joins a month',
    body: `QueueUp has a free plan.

Ten joins a month from your phone, plus one free wipe join: your PC watches the server through the restart and queues the moment it comes back.

Nothing to pay and no card needed. Sign in and tap Join: https://queueuprust.com/servers

If you want more, Unlimited is £1.99 for your first month, then £4.99 a month.`,
  },
  {
    label: 'Link your PC',
    segment: 'free_no_pc',
    subject: 'Two minutes and QueueUp is ready',
    body: `You made a QueueUp account but haven't linked your PC yet.

It takes about two minutes, once: download the small app on your gaming PC and type the code it shows into your phone.

https://queueuprust.com/get

After that, joining a Rust server is one tap from wherever you are.`,
  },
  {
    label: 'Come back',
    segment: 'lapsed',
    subject: 'QueueUp has a free plan now',
    body: `You used to be on QueueUp Unlimited. Since then there's a free plan: ten joins a month and no card needed, so your account still works.

Your PC is probably still linked. Sign in and tap Join: https://queueuprust.com/servers`,
  },
  {
    label: 'Phone app is out',
    segment: 'everyone',
    subject: 'QueueUp is on iPhone and Android',
    body: `QueueUp now has its own app. Join from your phone without opening a browser, and see your queue progress at a glance.

iPhone: (App Store link)
Android: (Google Play link)

Sign in with the account you already have.`,
  },
]

const STATUS: Record<Campaign['status'], string> = {
  draft: 'Draft',
  scheduled: 'Sending',
  done: 'Finished',
  cancelled: 'Cancelled',
}

const TEST_KEY = 'queueup_admin_test_email'

export default function Campaigns({ token }: { token: string }) {
  const [data, setData] = useState<Overview | null>(null)
  const [editing, setEditing] = useState<Partial<Campaign> | null>(null)
  const [testTo, setTestTo] = useState('')
  const [startAt, setStartAt] = useState('')
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    try {
      setTestTo(localStorage.getItem(TEST_KEY) ?? '')
    } catch {}
  }, [])

  const call = useCallback(
    async (path: string, body?: unknown) => {
      const res = await fetch('/api/relay' + path, {
        method: body === undefined ? 'GET' : 'POST',
        headers: { 'x-admin-token': token, ...(body !== undefined ? { 'content-type': 'application/json' } : {}) },
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      const out = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(out.error ?? `Relay said ${res.status}.`)
      return out
    },
    [token],
  )

  const load = useCallback(async () => {
    try {
      setData(await call('/admin/campaigns'))
    } catch (e) {
      setError((e as Error).message)
    }
  }, [call])

  useEffect(() => {
    if (token) load()
  }, [token, load])

  async function run(what: () => Promise<unknown>, done: string) {
    setBusy(true)
    setError('')
    setNote('')
    try {
      await what()
      setNote(done)
      await load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function save() {
    if (!editing) return
    await run(async () => {
      const saved: Campaign = await call(editing.id ? `/admin/campaigns/${editing.id}` : '/admin/campaigns', editing)
      setEditing(saved)
    }, 'Saved as a draft.')
  }

  if (!data) return error ? <div className="card"><p className="error">{error}</p></div> : null

  const sizeOf = (id?: string) => data.segments.find((s) => s.id === id)?.size ?? 0
  const nameOf = (id?: string) => data.segments.find((s) => s.id === id)?.name ?? id

  return (
    <div className="card">
      <h2>Email</h2>
      {!data.email_ready && (
        <p className="error">Email isn&apos;t switched on for the relay, so nothing below will send.</p>
      )}

      <h3 style={{ margin: '4px 0 8px', fontSize: 15 }}>Automatic, last 30 days</h3>
      <div className="stack" style={{ gap: 6 }}>
        {FLOWS.map((f) => (
          <div key={f.key} className="row" style={{ justifyContent: 'space-between', alignItems: 'baseline' }}>
            <span>
              {f.name} <span className="muted small">· {f.when}</span>
            </span>
            <b>{data.flows_30_days[f.key] ?? 0}</b>
          </div>
        ))}
      </div>

      <h3 style={{ margin: '20px 0 8px', fontSize: 15 }}>Campaigns</h3>
      {note && <p className="notice">{note}</p>}
      {error && <p className="error">{error}</p>}

      {data.campaigns.length === 0 && !editing && <p className="muted">None yet.</p>}
      <div className="stack">
        {data.campaigns.map((c) => (
          <div key={c.id} className="row" style={{ justifyContent: 'space-between', flexWrap: 'wrap', gap: 8 }}>
            <span style={{ minWidth: 0 }}>
              <b>{c.subject}</b>
              <br />
              <span className="muted small">
                {nameOf(c.segment)} · {c.sent} sent
                {c.status === 'scheduled' ? ` · ${c.daily_limit} a day` : ''}
              </span>
            </span>
            <span className="row" style={{ gap: 6 }}>
              <span className={`pill ${c.status === 'scheduled' ? 'good' : c.status === 'cancelled' ? 'bad' : ''}`}>
                {STATUS[c.status]}
              </span>
              {c.status === 'draft' && (
                <button className="quiet" onClick={() => setEditing(c)}>Open</button>
              )}
              {c.status === 'scheduled' && (
                <button
                  className="quiet"
                  disabled={busy}
                  onClick={() => run(() => call(`/admin/campaigns/${c.id}/cancel`, {}), 'Stopped. Nothing more will send.')}
                >
                  Stop
                </button>
              )}
            </span>
          </div>
        ))}
      </div>

      {!editing ? (
        <div className="stack" style={{ marginTop: 14 }}>
          <label>Start a new campaign from</label>
          <div className="row" style={{ flexWrap: 'wrap', gap: 6 }}>
            {TEMPLATES.map((t) => (
              <button
                key={t.label}
                onClick={() => setEditing({ subject: t.subject, body: t.body, segment: t.segment, daily_limit: 200 })}
              >
                {t.label}
              </button>
            ))}
            <button onClick={() => setEditing({ subject: '', body: '', segment: 'everyone', daily_limit: 200 })}>
              Blank
            </button>
          </div>
        </div>
      ) : (
        <div className="stack" style={{ marginTop: 14 }}>
          <div>
            <label htmlFor="cmp-seg">Who gets it</label>
            <select
              id="cmp-seg"
              value={editing.segment}
              onChange={(e) => setEditing({ ...editing, segment: e.target.value })}
              style={selectStyle}
            >
              {data.segments.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name} ({s.size})
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="cmp-subject">Subject</label>
            <input
              id="cmp-subject"
              value={editing.subject ?? ''}
              onChange={(e) => setEditing({ ...editing, subject: e.target.value })}
            />
          </div>
          <div>
            <label htmlFor="cmp-body">Email</label>
            <textarea
              id="cmp-body"
              value={editing.body ?? ''}
              onChange={(e) => setEditing({ ...editing, body: e.target.value })}
              style={{ minHeight: 220 }}
            />
            <p className="muted small" style={{ margin: '6px 0 0' }}>
              Plain text. The unsubscribe link is added for you.
            </p>
          </div>
          <div>
            <label htmlFor="cmp-limit">Most to send in one day</label>
            <input
              id="cmp-limit"
              type="number"
              min={1}
              value={editing.daily_limit ?? 200}
              onChange={(e) => setEditing({ ...editing, daily_limit: Number(e.target.value) })}
            />
            <p className="muted small" style={{ margin: '6px 0 0' }}>
              {Math.max(1, Math.ceil(sizeOf(editing.segment) / Math.max(1, editing.daily_limit ?? 200)))} day(s) to
              reach all {sizeOf(editing.segment)}. Keep the first big send at a few hundred a day so inboxes
              learn to trust the address.
            </p>
          </div>

          <button className="primary btn-wide" disabled={busy} onClick={save}>
            Save draft
          </button>

          {editing.id && (
            <>
              <div className="row" style={{ gap: 6 }}>
                <input
                  type="email"
                  placeholder="you@example.com"
                  value={testTo}
                  onChange={(e) => setTestTo(e.target.value)}
                  aria-label="Send a test to"
                />
                <button
                  disabled={busy || !testTo}
                  onClick={() =>
                    run(async () => {
                      try { localStorage.setItem(TEST_KEY, testTo) } catch {}
                      await call(`/admin/campaigns/${editing.id}/test`, { to: testTo })
                    }, `Test sent to ${testTo}. Read it on your phone before you send it to anyone.`)
                  }
                >
                  Send me a test
                </button>
              </div>
              <div>
                <label htmlFor="cmp-start">Start sending (leave empty for now)</label>
                <input
                  id="cmp-start"
                  type="datetime-local"
                  value={startAt}
                  onChange={(e) => setStartAt(e.target.value)}
                />
              </div>
              <button
                className="btn-wide"
                disabled={busy}
                onClick={() => {
                  const n = sizeOf(editing.segment)
                  if (!window.confirm(`Send "${editing.subject}" to ${n} people? The words can't be changed after this.`)) return
                  run(async () => {
                    await call(`/admin/campaigns/${editing.id}/schedule`, {
                      start_at: startAt ? new Date(startAt).toISOString() : undefined,
                    })
                    setEditing(null)
                    setStartAt('')
                  }, 'Scheduled. It sends a batch every ten minutes, up to the daily limit.')
                }}
              >
                Schedule it
              </button>
            </>
          )}
          <button className="quiet" onClick={() => setEditing(null)}>
            Close
          </button>
        </div>
      )}
    </div>
  )
}

const selectStyle: React.CSSProperties = {
  width: '100%',
  padding: '12px 14px',
  borderRadius: 14,
  border: '1px solid var(--line)',
  background: 'var(--card)',
  color: 'var(--text)',
  fontSize: 16,
}
