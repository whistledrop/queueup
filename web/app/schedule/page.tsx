'use client'

import { Suspense, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { api, type Device } from '@/lib/api'
import type { Favourite } from '@/lib/types'

export default function SchedulePage() {
  return (
    <Suspense>
      <ScheduleForm />
    </Suspense>
  )
}

function ScheduleForm() {
  const router = useRouter()
  const params = useSearchParams()

  const [device, setDevice] = useState<Device | null>(null)
  const [favourites, setFavourites] = useState<Favourite[]>([])
  const [serverId, setServerId] = useState(params.get('server_id') ?? '')
  const [serverName, setServerName] = useState(params.get('name') ?? '')
  const [when, setWhen] = useState('')
  const [waitForUp, setWaitForUp] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api<{ devices: Device[] }>('/api/devices')
      .then((d) => setDevice(d.devices?.[0] ?? null))
      .catch(() => {})
    api<{ favourites: Favourite[] }>('/api/favourites')
      .then((f) => setFavourites(f.favourites ?? []))
      .catch(() => {})
  }, [])

  // A sensible default: the next first-Thursday-of-the-month at 19:00 UK time
  // would be presumptuous; just default to one hour from now, rounded up.
  useEffect(() => {
    if (when) return
    const d = new Date(Date.now() + 60 * 60 * 1000)
    d.setMinutes(0, 0, 0)
    setWhen(toLocalInput(d))
  }, [when])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!device) return
    setBusy(true)
    setError('')
    try {
      // The input is the user's local wall-clock time. new Date() reads it in
      // the phone's timezone, and toISOString converts to UTC. The relay only
      // ever sees UTC; Spain phone and UK PC agree by construction.
      const fireAt = new Date(when)
      if (isNaN(fireAt.getTime())) throw new Error('Pick a date and time.')
      await api('/api/schedules', {
        method: 'POST',
        body: JSON.stringify({
          device_id: device.id,
          server_id: serverId,
          server_name: serverName,
          fire_at: fireAt.toISOString(),
          wait_for_server_up: waitForUp,
        }),
      })
      router.push('/')
      router.refresh()
    } catch (err) {
      setError((err as Error).message)
      setBusy(false)
    }
  }

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn small" style={{ minHeight: 36, padding: '6px 12px' }}>
          Back
        </Link>
      </header>

      {error && <div className="error">{error}</div>}
      {!device && (
        <div className="notice">Link your PC first, then you can schedule joins.</div>
      )}

      <form onSubmit={submit}>
        <div className="card">
          <h2>Which server</h2>
          {serverId ? (
            <div className="row">
              <div>
                <div style={{ fontWeight: 600 }}>{serverName || serverId}</div>
                <div className="muted small">From your search</div>
              </div>
              <Link href="/servers" className="btn small" style={{ minHeight: 36, padding: '6px 12px' }}>
                Change
              </Link>
            </div>
          ) : favourites.length > 0 ? (
            <div className="stack">
              {favourites.map((f) => (
                <button
                  type="button"
                  key={f.server_id}
                  onClick={() => { setServerId(f.server_id); setServerName(f.name) }}
                >
                  {f.name}
                </button>
              ))}
              <Link href="/servers" className="btn">Search instead</Link>
            </div>
          ) : (
            <Link href="/servers" className="btn btn-wide">Find a server</Link>
          )}
        </div>

        <div className="card">
          <h2>When</h2>
          <label htmlFor="when">Date and time (your local time)</label>
          <input
            id="when"
            type="datetime-local"
            value={when}
            onChange={(e) => setWhen(e.target.value)}
            required
          />
          <p className="muted small" style={{ marginBottom: 0 }}>
            Your PC acts on this exact moment wherever it is. Set 8pm from Spain
            and a PC in the UK joins at 7pm its time, which is the same instant.
          </p>
        </div>

        <div className="card">
          <h2>Wipe mode</h2>
          <label className="row" style={{ cursor: 'pointer', marginBottom: 0 }}>
            <span>
              <span style={{ color: 'var(--text)', fontWeight: 600 }}>
                Wait for the server to come back up
              </span>
              <span className="muted small" style={{ display: 'block' }}>
                For wipe day. From the scheduled time, your PC watches the server
                through its restart and connects the moment it returns.
              </span>
            </span>
            <input
              type="checkbox"
              checked={waitForUp}
              onChange={(e) => setWaitForUp(e.target.checked)}
              style={{ width: 24, minHeight: 24 }}
            />
          </label>
        </div>

        <button
          type="submit"
          className="primary btn-wide"
          disabled={busy || !device || !serverId || !when}
        >
          {busy ? 'Saving' : 'Schedule the join'}
        </button>
      </form>
    </div>
  )
}

function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}
