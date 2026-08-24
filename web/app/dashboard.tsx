'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { api, getBilling, isActive, outcome, stateLabel, type Billing, type Device, type Job } from '@/lib/api'
import type { Favourite, Schedule } from '@/lib/types'
import { disablePush, enablePush, pushState, sendTestPush, type PushState } from '@/lib/push'

function siteHost(): string {
  if (typeof window === 'undefined') return 'queueup'
  return window.location.host
}

export default function Dashboard({ email }: { email: string }) {
  const router = useRouter()
  const [devices, setDevices] = useState<Device[]>([])
  const [jobs, setJobs] = useState<Job[]>([])
  const [favourites, setFavourites] = useState<Favourite[]>([])
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [billing, setBilling] = useState<Billing | null>(null)
  const [push, setPush] = useState<PushState>('unsupported')
  const [pushBusy, setPushBusy] = useState(false)
  const [pushNote, setPushNote] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [code, setCode] = useState('')
  const [pairing, setPairing] = useState(false)
  const [joining, setJoining] = useState('')

  const load = useCallback(async () => {
    try {
      const [d, j, f, sc] = await Promise.all([
        api<{ devices: Device[] }>('/api/devices'),
        api<{ jobs: Job[] }>('/api/jobs?limit=8'),
        api<{ favourites: Favourite[] }>('/api/favourites'),
        api<{ schedules: Schedule[] }>('/api/schedules'),
      ])
      setDevices(d.devices ?? [])
      setJobs(j.jobs ?? [])
      setFavourites(f.favourites ?? [])
      setSchedules(sc.schedules ?? [])
      setError('')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    load()
    // Keep the PC's online light honest without the user pulling to refresh.
    const t = setInterval(load, 5000)
    pushState().then(setPush).catch(() => {})
    getBilling().then(setBilling).catch(() => {})
    return () => clearInterval(t)
  }, [load])

  // The gate, stated up front so the paywall is never a surprise later.
  const needsSub = billing !== null && billing.enabled && !billing.subscribed

  async function togglePush() {
    setPushBusy(true)
    setPushNote('')
    try {
      if (push === 'on') {
        await disablePush()
      } else {
        await enablePush()
      }
      setPush(await pushState())
    } catch (e) {
      setPushNote((e as Error).message)
    } finally {
      setPushBusy(false)
    }
  }

  async function testPush() {
    setPushBusy(true)
    setPushNote('')
    try {
      await sendTestPush()
      setPushNote('Sent. It should pop up on this device in a moment.')
    } catch (e) {
      setPushNote((e as Error).message)
    } finally {
      setPushBusy(false)
    }
  }

  async function cancelSchedule(id: string) {
    try {
      await api(`/api/schedules/${id}/cancel`, { method: 'POST' })
      await load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const pc = devices[0]
  const active = jobs.find((j) => isActive(j.state))

  async function pair(e: React.FormEvent) {
    e.preventDefault()
    setPairing(true)
    setError('')
    try {
      await api('/api/pair', { method: 'POST', body: JSON.stringify({ code: code.trim().toUpperCase() }) })
      setCode('')
      await load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setPairing(false)
    }
  }

  async function join(serverId: string, name: string) {
    if (!pc) return
    if (needsSub) {
      router.push('/subscribe')
      return
    }
    setJoining(serverId)
    setError('')
    try {
      const job = await api<Job>('/api/jobs', {
        method: 'POST',
        body: JSON.stringify({ device_id: pc.id, server_id: serverId, server_name: name }),
      })
      router.push(`/jobs/${job.id}`)
    } catch (e) {
      setError((e as Error).message)
      setJoining('')
    }
  }

  async function signOut() {
    await fetch('/api/auth/logout', { method: 'POST' })
    router.push('/login')
    router.refresh()
  }

  return (
    <>
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <button className="small" onClick={signOut} style={{ minHeight: 36, padding: '6px 12px' }}>
          Sign out
        </button>
      </header>

      {error && <div className="error">{error}</div>}

      {active && (
        <Link href={`/jobs/${active.id}`} className="card" style={{ display: 'block', textDecoration: 'none' }}>
          <h2>Happening now</h2>
          <div className="row">
            <div>
              <div style={{ fontWeight: 700, fontSize: 18 }}>{stateLabel(active.state)}</div>
              <div className="muted">{active.server_name || active.server_addr}</div>
            </div>
            {active.state === 'queued' && active.position > 0 && (
              <div style={{ fontSize: 30, fontWeight: 800 }}>{active.position}</div>
            )}
          </div>
        </Link>
      )}

      <div className="card">
        <h2>Your PC</h2>
        {loading && !pc && <div className="muted">Loading</div>}

        {!loading && !pc && (
          <>
            <p className="muted" style={{ marginTop: 0 }}>
              Do this once, on the gaming PC you want QueueUp to use.
            </p>

            <ol className="setup">
              <li>
                <strong>Download the QueueUp agent on that PC.</strong>
                <span className="muted small">
                  Open this page in a browser on the PC itself, not on your phone.
                </span>
                <a className="btn btn-primary" href="/download" style={{ marginTop: 8 }}>
                  Download for Windows
                </a>
                <span className="muted small" style={{ marginTop: 8, display: 'block' }}>
                  Or type this into the PC&apos;s browser:{' '}
                  <span className="mono">{siteHost()}/download</span>
                </span>
              </li>
              <li>
                <strong>Put it somewhere permanent and double-click it.</strong>
                <span className="muted small">
                  A folder like C:\QueueUp is ideal. Not Downloads: it lives there
                  from now on.
                </span>
                <SmartScreenHelp />
              </li>
              <li>
                <strong>Type the six character code it shows, here.</strong>
              </li>
            </ol>

            <form onSubmit={pair} className="stack" style={{ marginTop: 14 }}>
              <input
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="ABC123"
                maxLength={6}
                autoCapitalize="characters"
                autoCorrect="off"
                spellCheck={false}
                aria-label="Pairing code"
              />
              <button type="submit" className="primary btn-wide" disabled={pairing || code.length < 6}>
                {pairing ? 'Linking' : 'Link this PC'}
              </button>
            </form>

            <p className="muted small" style={{ marginBottom: 0 }}>
              Stuck? The code lasts ten minutes. Close the black window and
              double-click the agent again for a fresh one.
            </p>
          </>
        )}

        {pc && (
          <>
            <div className="row">
              <div>
                <div style={{ fontWeight: 600 }}>
                  <span className={`dot ${pc.online ? 'on' : 'off'}`} />
                  {pc.name}
                </div>
                <div className="muted small">
                  {pc.online ? 'Online and ready' : 'Offline. Joins will start when it comes back.'}
                </div>
              </div>
              {pc.simulator && <span className="pill warn">Simulator</span>}
            </div>
          </>
        )}
        {needsSub && (
          <p className="muted small" style={{ marginBottom: 0 }}>
            Setting up is free. Joining needs the subscription, {billing?.price_line}.
          </p>
        )}
      </div>

      <div className="card">
        <h2>Join a server</h2>
        <Link href="/servers" className="btn btn-primary btn-wide">Find a server</Link>

        {favourites.length > 0 && (
          <div style={{ marginTop: 14 }}>
            {favourites.map((f) => (
              <div className="server" key={f.server_id}>
                <div className="row">
                  <div style={{ minWidth: 0 }}>
                    <div className="name">{f.name}</div>
                    <div className="muted small">{f.address}</div>
                  </div>
                  <button
                    className="primary"
                    disabled={!pc || !!active || joining === f.server_id}
                    onClick={() => join(f.server_id, f.name)}
                  >
                    {joining === f.server_id ? '...' : 'Join'}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="card">
        <h2>Scheduled joins</h2>
        <Link href="/schedule" className="btn btn-wide">Schedule a join</Link>
        {schedules.filter((sc) => sc.state === 'pending').map((sc) => (
          <div className="server" key={sc.id}>
            <div className="row">
              <div style={{ minWidth: 0 }}>
                <div className="name">{sc.server_name || sc.server_addr}</div>
                <div className="muted small">
                  {new Date(sc.fire_at).toLocaleString()}
                  {sc.wait_for_server_up && ' - waits for the wipe restart'}
                </div>
              </div>
              <button onClick={() => cancelSchedule(sc.id)} style={{ minHeight: 40, padding: '6px 12px' }}>
                Cancel
              </button>
            </div>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>Notifications</h2>
        {push === 'unsupported' && (
          <p className="muted small" style={{ margin: 0 }}>
            This browser can't show push notifications. On iPhone, add QueueUp to
            your home screen first (Share, then Add to Home Screen).
          </p>
        )}
        {push === 'relay-off' && (
          <p className="muted small" style={{ margin: 0 }}>
            Notifications aren't set up on the relay yet. See the README.
          </p>
        )}
        {push === 'denied' && (
          <p className="muted small" style={{ margin: 0 }}>
            Notifications are blocked for this site in your browser settings.
          </p>
        )}
        {(push === 'off' || push === 'on') && (
          <div className="spread">
            <button onClick={togglePush} disabled={pushBusy} className={push === 'on' ? '' : 'primary'}>
              {push === 'on' ? 'Turn off' : 'Turn on notifications'}
            </button>
            {push === 'on' && (
              <button onClick={testPush} disabled={pushBusy}>Send a test</button>
            )}
          </div>
        )}
        {pushNote && <p className="muted small" style={{ marginBottom: 0 }}>{pushNote}</p>}
      </div>

      {jobs.length > 0 && (
        <div className="card">
          <h2>Recent</h2>
          {jobs.slice(0, 5).map((j) => (
            <div className="server" key={j.id}>
              <Link href={`/jobs/${j.id}`} className="row" style={{ textDecoration: 'none' }}>
                <div style={{ minWidth: 0 }}>
                  <div className="name">{j.server_name || j.server_addr}</div>
                  <div className="muted small">{j.detail || stateLabel(j.state)}</div>
                </div>
                <span className={`pill ${outcome(j).tone}`}>{outcome(j).label}</span>
              </Link>
            </div>
          ))}
        </div>
      )}

      <p className="muted small" style={{ textAlign: 'center' }}>{email}</p>
    </>
  )
}

/* Windows blocks apps it has not seen before, and QueueUp is new, so every
   customer meets this dialog once. The button they need is a small grey text
   link that does not look like a button, and the whole thing reads like a virus
   warning, so people stop here. Showing them the dialog before they see it,
   with the two clicks numbered, turns a scare into a formality. */
function SmartScreenHelp() {
  return (
    <div className="warnBox">
      <h4>Windows will try to stop you. This is expected.</h4>
      <p>
        QueueUp is new, so Windows does not recognise it yet. You will see this
        exact box. Here is what to press.
      </p>

      <div className="winDialog">
        <div className="winTitle">Windows protected your PC</div>
        <div className="winBody">
          Microsoft Defender SmartScreen prevented an unrecognised app from
          starting. Running this app might put your PC at risk.
        </div>
        <div className="winRow">
          <span className="winLink">More info</span>
          <span className="winBtn">Don&apos;t run</span>
        </div>
      </div>

      <div className="clickStep">
        <span className="clickOrder">1</span>
        <span>
          Click <strong>More info</strong>. It is the small underlined text on
          the left, not a button. This is the bit everyone misses.
        </span>
      </div>

      <div className="winDialog" style={{ marginTop: 10 }}>
        <div className="winTitle">Windows protected your PC</div>
        <div className="winBody">
          App: QueueUpAgent.exe
          <br />
          Publisher: Unknown publisher
        </div>
        <div className="winRow" style={{ justifyContent: 'flex-end' }}>
          <span className="winBtn marked">Run anyway</span>
          <span className="winBtn">Don&apos;t run</span>
        </div>
      </div>

      <div className="clickStep">
        <span className="clickOrder">2</span>
        <span>
          A <strong>Run anyway</strong> button appears. Click it. You only ever
          do this once.
        </span>
      </div>
    </div>
  )
}
