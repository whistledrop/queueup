'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api, isActive, type Device, type Job, type ServerInfo } from '@/lib/api'

export default function ServersPage() {
  const router = useRouter()
  const [query, setQuery] = useState('')
  const [servers, setServers] = useState<ServerInfo[]>([])
  const [source, setSource] = useState('')
  const [device, setDevice] = useState<Device | null>(null)
  const [busyJob, setBusyJob] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [joining, setJoining] = useState('')

  const search = useCallback(async (q: string) => {
    setLoading(true)
    try {
      const res = await api<{ source: string; servers: ServerInfo[] }>(
        `/api/servers/search?q=${encodeURIComponent(q)}&limit=25`,
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
  }, [])

  // Search as you type, but wait for a pause so every keystroke is not a request.
  useEffect(() => {
    const t = setTimeout(() => search(query), 300)
    return () => clearTimeout(t)
  }, [query, search])

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
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn small" style={{ minHeight: 36, padding: '6px 12px' }}>
          Back
        </Link>
      </header>

      {error && <div className="error">{error}</div>}

      {source === 'stub' && (
        <div className="notice">
          Showing the built-in example list. Real server search needs a key on the
          relay. See the README.
        </div>
      )}

      {!device && (
        <div className="notice">Link your PC first, then you can join from here.</div>
      )}
      {busyJob && (
        <div className="notice">Your PC is already working on a join. Cancel it before starting another.</div>
      )}

      <div className="card">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search servers by name"
          aria-label="Search servers by name"
          autoCorrect="off"
          spellCheck={false}
        />
      </div>

      <div className="card">
        <h2>
          {loading ? 'Searching' : `${servers.length} server${servers.length === 1 ? '' : 's'}`}
        </h2>
        {!loading && servers.length === 0 && (
          <div className="muted">Nothing matched that. Try a shorter search.</div>
        )}
        {servers.map((s) => (
          <div className="server" key={s.id}>
            <div className="row">
              <div style={{ minWidth: 0 }}>
                <div className="name">{s.name}</div>
                <div className="muted small">
                  {s.online ? `${s.players} / ${s.max_players} players` : 'Offline'}
                  {s.queue > 0 && `, ${s.queue} in queue`}
                  {s.region && `, ${s.region}`}
                </div>
              </div>
              <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                <button
                  onClick={() => toggleFavourite(s)}
                  aria-label={s.favourite ? 'Remove from saved' : 'Save this server'}
                  style={{ minHeight: 40, padding: '6px 12px' }}
                >
                  {s.favourite ? 'Saved' : 'Save'}
                </button>
                <Link
                  href={`/schedule?server_id=${encodeURIComponent(s.id)}&name=${encodeURIComponent(s.name)}`}
                  className="btn"
                  style={{ minHeight: 40, padding: '6px 12px' }}
                >
                  Later
                </Link>
                <button
                  className="primary"
                  disabled={!device || busyJob || joining === s.id}
                  onClick={() => join(s)}
                  style={{ minHeight: 40, padding: '6px 14px' }}
                >
                  {joining === s.id ? '...' : 'Join'}
                </button>
              </div>
            </div>
          </div>
        ))}
      </div>
    </>
  )
}
