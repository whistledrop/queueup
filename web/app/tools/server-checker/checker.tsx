'use client'

// Search a Rust server and check it live. Everything comes from QueueUp's
// relay, which asks Steam's public list and then the server itself.

import Link from 'next/link'
import { useState } from 'react'
import s from '../tools.module.css'

type Found = {
  id: string
  name: string
  address: string
  online: boolean
  players: number
  max_players: number
  queue: number
  map?: string
}

type Status = Found & { answered?: boolean; up_since?: string; checked_at?: string }

async function relay<T>(path: string): Promise<T> {
  const res = await fetch('/api/relay' + path)
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error ?? 'That did not work. Try again in a moment.')
  return body as T
}

function since(iso: string): string {
  const mins = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000))
  if (mins < 60) return `${mins} minute${mins === 1 ? '' : 's'}`
  const hours = Math.round(mins / 60)
  if (hours < 48) return `${hours} hour${hours === 1 ? '' : 's'}`
  const days = Math.round(hours / 24)
  return `${days} days`
}

export default function Checker() {
  const [q, setQ] = useState('')
  const [found, setFound] = useState<Found[] | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function search(e: React.FormEvent) {
    e.preventDefault()
    if (q.trim().length < 2 || busy) return
    setBusy(true)
    setError('')
    setStatus(null)
    try {
      const out = await relay<{ servers: Found[] }>(`/api/public/servers?q=${encodeURIComponent(q.trim())}`)
      setFound(out.servers)
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  async function check(sv: Found) {
    setBusy(true)
    setError('')
    try {
      setStatus(await relay<Status>(`/api/public/servers/${encodeURIComponent(sv.id)}`))
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={s.panel}>
      <form className={s.field} onSubmit={search}>
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Server name, e.g. Rustoria"
          aria-label="Server name"
        />
        <button className={s.btn} disabled={busy || q.trim().length < 2}>
          {busy && !status ? 'Searching' : 'Search'}
        </button>
      </form>
      {error && <p className={s.error}>{error}</p>}

      {status ? (
        <div>
          <h2 style={{ margin: '18px 0 4px', fontSize: 20, fontWeight: 600 }}>{status.name}</h2>
          <p className={s.note} style={{ marginTop: 0 }}>
            {status.address}
            {status.map ? ` · ${status.map}` : ''}
          </p>
          <p style={{ margin: '12px 0 0' }}>
            {status.online ? <span className={s.up}>● Online</span> : <span className={s.down}>● Not answering</span>}
          </p>
          {status.online && (
            <div className={s.stats}>
              <div className={s.stat}><b>{status.players}/{status.max_players}</b><span>players</span></div>
              <div className={s.stat}><b>{status.queue}</b><span>in the queue</span></div>
              {status.up_since && (
                <div className={s.stat}><b>{since(status.up_since)}</b><span>since it last started</span></div>
              )}
            </div>
          )}
          {status.online && status.up_since && (
            <p className={s.note}>
              A server restarts when it wipes, so a short time here usually means a fresh wipe. Some
              servers also restart without wiping, so check their Discord to be sure.
            </p>
          )}
          {!status.online && (
            <p className={s.note}>
              It didn&apos;t answer just now. That usually means it is restarting, often for a wipe,
              or is down. Check again in a few minutes.
            </p>
          )}
          <div className={s.row}>
            <button className={s.ghost} onClick={() => check(status)} disabled={busy}>
              {busy ? 'Checking' : 'Check again'}
            </button>
            <button className={s.ghost} onClick={() => setStatus(null)}>Back to results</button>
          </div>
          <div className={s.panel} style={{ margin: '18px 0 0', background: 'var(--card-2)' }}>
            <p style={{ margin: '0 0 12px', lineHeight: 1.55 }}>
              {status.queue > 0
                ? `${status.queue} people are queuing for this server. QueueUp can queue for you from your phone while your PC waits at home.`
                : 'Want in when it wipes? QueueUp can join this server from your phone, or the moment it comes back on wipe night.'}
            </p>
            <Link href="/" className={s.btn} style={{ textDecoration: 'none', display: 'inline-block' }}>
              Try QueueUp free
            </Link>
          </div>
        </div>
      ) : (
        found && (
          found.length === 0 ? (
            <p className={s.note}>No Rust servers by that name in the public list right now.</p>
          ) : (
            <ul className={s.results}>
              {found.map((sv) => (
                <li key={sv.id}>
                  <button onClick={() => check(sv)} disabled={busy}>
                    {sv.name}
                    <small>
                      {sv.online ? `${sv.players}/${sv.max_players} players` : 'offline'}
                      {sv.queue > 0 ? ` · ${sv.queue} queued` : ''} · {sv.address}
                    </small>
                  </button>
                </li>
              ))}
            </ul>
          )
        )
      )}
    </div>
  )
}
