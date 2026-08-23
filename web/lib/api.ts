'use client'

// Small wrapper the pages use. Every call goes to this app, which adds the
// session token and forwards it to the relay.

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch('/api/relay' + path, {
    ...init,
    headers: init?.body ? { 'content-type': 'application/json' } : undefined,
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok) {
    throw new Error(body.error ?? 'Something went wrong. Try again.')
  }
  return body as T
}

export type Device = {
  id: string
  name: string
  online: boolean
  agent_version: string
  os: string
  simulator: boolean
  last_seen_at: string
}

export type Job = {
  id: string
  device_id: string
  device_online: boolean
  server_addr: string
  server_name: string
  server_id: string
  state: string
  position: number
  attempt: number
  detail: string
  reason_code: string
  reason_message: string
  created_at: string
  updated_at: string
}

export type ServerInfo = {
  id: string
  name: string
  address: string
  online: boolean
  players: number
  max_players: number
  queue: number
  region: string
  favourite: boolean
}

export type JobEvent = {
  id: number
  state: string
  position: number
  detail: string
  at: string
}

/** Plain words for a job state. The relay's detail text is preferred when set. */
export function stateLabel(state: string): string {
  switch (state) {
    case 'pending': return 'Waiting for your PC'
    case 'waiting_for_server_up': return 'Waiting for the server'
    case 'launching': return 'Launching Rust'
    case 'connecting': return 'Connecting'
    case 'queued': return 'In the queue'
    case 'in_server': return "You're in"
    case 'retrying': return 'Trying again'
    case 'done': return 'Finished'
    case 'failed': return 'Did not work'
    default: return state
  }
}

export function isActive(state: string): boolean {
  return state !== 'done' && state !== 'failed'
}

/** How a finished job should be summed up. Cancelling is not the same as
 *  getting in, and they must not look the same on the screen. */
export function outcome(job: { state: string; reason_code: string }): {
  label: string
  tone: 'good' | 'bad' | 'warn' | ''
} {
  if (job.state === 'failed') return { label: 'Did not work', tone: 'bad' }
  if (job.state === 'done') {
    return job.reason_code === 'cancelled'
      ? { label: 'Cancelled', tone: '' }
      : { label: 'Joined', tone: 'good' }
  }
  return { label: stateLabel(job.state), tone: 'warn' }
}
