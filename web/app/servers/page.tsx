'use client'

import { Suspense, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import Nav, { Footer } from '../nav'
import { api, isActive, type Device, type Job, type ServerInfo } from '@/lib/api'
import type { Schedule } from '@/lib/types'

// Quick ways in, for people who don't arrive with a server name to type.
// Each chip is just a search, so the relay needs nothing new to support them.
const chips = [
  { label: 'Busiest', q: '' },
  { label: 'EU', q: 'EU' },
  { label: 'US', q: 'US' },
  { label: 'Vanilla', q: 'vanilla' },
  { label: 'Solo/Duo/Trio', q: 'trio' },
  { label: '2x', q: '2x' },
]

// useSearchParams needs a boundary for prerendering, same as the schedule page.
export default function ServersPage() {
  return (
    <Suspense>
      <ServerBrowser />
    </Suspense>
  )
}

function ServerBrowser() {
  const router = useRouter()
  // Arriving from the schedule flow means picking a server for LATER, not
  // joining one now. Same list, different verb, and it hands the choice back.
  const forSchedule = useSearchParams().get('for') === 'schedule'
  const [query, setQuery] = useState('')
  const [typed, setTyped] = useState('')
  const [servers, setServers] = useState<ServerInfo[]>([])
  const [source, setSource] = useState('')
  const [device, setDevice] = useState<Device | null>(null)
  const [busyJob, setBusyJob] = useState(false)
  const [scheduled, setScheduled] = useState<Schedule | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [joining, setJoining] = useState('')

  const search = useCallback(async (q: string) => {
    setLoading(true)
    try {
      const res = await api<{ source: string; servers: ServerInfo[] }>(
        `/api/servers/search?q=${encodeURIComponent(q)}&limit=100`,
      )
      setServers(res.servers ?? [])
      setSource(res.source)
      setError('')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    api<{ devices: Device[] }>('/api/devices')
      .then((d) => setDevice(d.devices?.[0] ?? null))
      .catch(() => {})
    api<{ jobs: Job[] }>('/api/jobs?limit=5')
      .then((j) => setBusyJob((j.jobs ?? []).some((x) => isActive(x.state))))
      .catch(() => {})
    api<{ schedules: Schedule[] }>('/api/schedules')
      .then((r) => setScheduled((r.schedules ?? []).find((x) => x.state === 'pending') ?? null))
      .catch(() => {})
  }, [])

  // Search as you type, but wait for a pause so every keystroke is not a request.
  useEffect(() => {
    const t = setTimeout(() => search(query), 300)
    return () => clearTimeout(t)
  }, [query, search])

  function pickChip(q: string) {
    setTyped('')
    setQuery(q)
  }

  async function toggleFavourite(s: ServerInfo) {
    try {
      if (s.favourite) {
        await api(`/api/favourites/${encodeURIComponent(s.id)}`, { method: 'DELETE' })
      } else {
        await api('/api/favourites', {
          method: 'POST',
          body: JSON.stringify({
            server_id: s.id, name: s.name, address: s.address, region: s.region,
          }),
        })
      }
      setServers((list) =>
        list.map((x) => (x.id === s.id ? { ...x, favourite: !x.favourite } : x)),
      )
    } catch (e) {
      setError((e as Error).message)
    }
  }

  function choose(s: ServerInfo) {
    router.push(
      `/schedule?server_id=${encodeURIComponent(s.id)}&name=${encodeURIComponent(s.name)}`,
    )
  }

  async function join(s: ServerInfo) {
    if (!device) return
    setJoining(s.id)
    setError('')
    try {
      const job = await api<Job>('/api/jobs', {
        method: 'POST',
        body: JSON.stringify({ device_id: device.id, server_id: s.id, server_name: s.name }),
      })
      router.push(`/jobs/${job.id}`)
    } catch (e) {
      setError((e as Error).message)
      setJoining('')
    }
  }

  return (
    <>
      <Nav />

      {error && <div className="error">{error}</div>}

      {source === 'stub' && (
        <div className="notice">
          Showing the built-in example list. Real server search needs a key on the
          relay. See the README.
        </div>
      )}

      {forSchedule && (
        <div className="notice">
          Pick the server for your scheduled join. You can set the time on the
          next screen.
        </div>
      )}
      {!device && !forSchedule && (
        <div className="notice">Link your PC first, then you can join from here.</div>
      )}
      {busyJob && !forSchedule && (
        <div className="notice">
          Your PC is already working on a join. Cancel it before starting another.
        </div>
      )}
      {scheduled && !forSchedule && (
        <div className="notice">
          <b>
            You have a join scheduled for {scheduled.server_name || scheduled.server_addr}
            {' '}on {new Date(scheduled.fire_at).toLocaleString()}.
          </b>{' '}
          Your PC can only do one at a time, so joining now would cost you that
          one. Cancel it on the{' '}
          <Link href="/schedule">Schedule</Link> page if you would rather play now.
        </div>
      )}

      <div className="card">
        <input
          value={typed}
          onChange={(e) => { setTyped(e.target.value); setQuery(e.target.value) }}
          placeholder="Search servers by name"
          aria-label="Search servers by name"
          autoCorrect="off"
          spellCheck={false}
        />
        <div className="chips" style={{ marginTop: 12, marginBottom: 0 }}>
          {chips.map((c) => (
            <button
              key={c.label}
              className={`chip ${query === c.q && typed === '' ? 'active' : ''}`}
              onClick={() => pickChip(c.q)}
            >
              {c.label}
            </button>
          ))}
        </div>
      </div>

      <div className="card">
        <h2>{loading ? 'Searching' : `${servers.length} server${servers.length === 1 ? '' : 's'}, busiest first`}</h2>
        {!loading && servers.length === 0 && (
          <div className="muted">Nothing matched that. Try a shorter search.</div>
        )}
        {servers.map((s) => {
          const pct = s.max_players > 0 ? Math.min(100, (s.players / s.max_players) * 100) : 0
          return (
            <div className="server" key={s.id}>
              <div className="row">
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="name" style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {s.name}
                  </div>
                  <div className="muted small">
                    {s.online ? `${s.players} / ${s.max_players} players` : 'Offline'}
                    {s.map && s.online && ` · ${s.map}`}
                  </div>
                  {s.online && (
                    <div className="fillbar">
                      <div className={pct >= 100 ? 'full' : ''} style={{ width: `${pct}%` }} />
                    </div>
                  )}
                </div>
                <div style={{ display: 'flex', gap: 4, alignItems: 'center', marginLeft: 10 }}>
                  {s.queue > 0 && <span className="pill warn">{s.queue} queued</span>}
                  <button
                    className={`star ${s.favourite ? 'on' : ''}`}
                    onClick={() => toggleFavourite(s)}
                    aria-label={s.favourite ? 'Remove from saved' : 'Save this server'}
                  >
                    {s.favourite ? '★' : '☆'}
                  </button>
                  {forSchedule ? (
                    <button
                      className="primary"
                      onClick={() => choose(s)}
                      style={{ minHeight: 40, padding: '6px 14px' }}
                    >
                      Choose
                    </button>
                  ) : (
                    <button
                      className="primary"
                      disabled={!device || busyJob || !!scheduled || joining === s.id}
                      onClick={() => join(s)}
                      style={{ minHeight: 40, padding: '6px 14px' }}
                    >
                      {joining === s.id ? '...' : 'Join'}
                    </button>
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>

      <Footer />
    </>
  )
}
