'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { api, isActive, outcome, stateLabel, type Job, type JobEvent } from '@/lib/api'

export default function LiveStatus({ jobId }: { jobId: string }) {
  const [job, setJob] = useState<Job | null>(null)
  const [events, setEvents] = useState<JobEvent[]>([])
  const [error, setError] = useState('')
  const [cancelling, setCancelling] = useState(false)
  const [connected, setConnected] = useState(false)
  const seen = useRef<Set<number>>(new Set())

  // The job itself, for the server name and the current state.
  useEffect(() => {
    let stop = false
    const load = () =>
      api<{ job: Job; events: JobEvent[] }>(`/api/jobs/${jobId}`)
        .then((r) => { if (!stop) setJob(r.job) })
        .catch((e) => { if (!stop) setError((e as Error).message) })
    load()
    const t = setInterval(load, 4000)
    return () => { stop = true; clearInterval(t) }
  }, [jobId])

  // The live feed. The browser reconnects this on its own if the phone drops off
  // the network, which on wipe day it will.
  useEffect(() => {
    const source = new EventSource(`/api/relay/api/jobs/${jobId}/events`)
    source.onopen = () => setConnected(true)
    source.onerror = () => setConnected(false)
    source.onmessage = (m) => {
      try {
        const e = JSON.parse(m.data) as JobEvent
        if (seen.current.has(e.id)) return
        seen.current.add(e.id)
        setEvents((list) => [...list, e])
      } catch {
        // ignore anything we cannot read
      }
    }
    return () => source.close()
  }, [jobId])

  async function cancel() {
    setCancelling(true)
    setError('')
    try {
      const updated = await api<Job>(`/api/jobs/${jobId}/cancel`, { method: 'POST' })
      setJob(updated)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setCancelling(false)
    }
  }

  const latest = events[events.length - 1]
  // The stream is usually the freshest view, but a job the relay says is
  // finished IS finished: a cancel must flip the screen at once, not wait for
  // the agent's confirming event to arrive.
  const state =
    job && isActive(job.state) === false ? job.state : (latest?.state ?? job?.state ?? 'pending')
  const position = state === 'queued' ? (latest?.position ?? job?.position ?? 0) : 0
  const running = isActive(state)

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn small" style={{ minHeight: 36, padding: '6px 12px' }}>
          Back
        </Link>
      </header>

      {error && <div className="error">{error}</div>}

      <div className="card">
        <h2>{job?.server_name || job?.server_addr || 'Join'}</h2>
        <p className="state">
          {running || !job ? stateLabel(state) : outcome(job).label}
        </p>
        <p className="muted" style={{ margin: 0 }}>
          {latest?.detail || job?.detail || ''}
        </p>

        {position > 0 && (
          <>
            <p className="position">{position}</p>
            <p className="muted" style={{ margin: 0 }}>your place in the queue, about</p>
          </>
        )}

        {state === 'in_server' && (
          <p className="muted" style={{ marginBottom: 0 }}>
            Your PC is holding the slot. Get to it when you can.
          </p>
        )}

        {job && !running && job.reason_message && (
          <p className="muted" style={{ marginBottom: 0 }}>{job.reason_message}</p>
        )}
      </div>

      {running && (
        <button className="danger btn-wide" onClick={cancel} disabled={cancelling}>
          {cancelling ? 'Stopping' : 'Cancel and close Rust'}
        </button>
      )}

      <div className="card" style={{ marginTop: 14 }}>
        <h2>
          What happened{' '}
          {running && (
            <span className="pill" style={{ textTransform: 'none', letterSpacing: 0 }}>
              {connected ? 'live' : 'reconnecting'}
            </span>
          )}
        </h2>
        <ul className="timeline">
          {events.map((e, i) => (
            <li key={e.id} className={i === events.length - 1 ? 'now' : ''}>
              <div>{e.detail || stateLabel(e.state)}</div>
              <div className="when">{new Date(e.at).toLocaleTimeString()}</div>
            </li>
          ))}
          {events.length === 0 && <li className="muted">Waiting for the first update</li>}
        </ul>
      </div>
    </div>
  )
}
