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
    // 402 means the subscription gate. Wherever it comes from, the answer is
    // the same page, so handle it once here rather than in every button.
    if (res.status === 402 && typeof window !== 'undefined') {
      window.location.href = '/subscribe'
    }
    throw new Error(body.error ?? 'Something went wrong. Try again.')
  }
  return body as T
}

export type Billing = {
  enabled: boolean
  subscribed: boolean
  price_line: string
}

/** The subscription gate's state. With billing off, everyone reads subscribed,
 *  so no paywall ever shows that cannot be honoured. */
export function getBilling(): Promise<Billing> {
  return api<Billing>('/api/billing')
}

export type Device = {
  id: string
  name: string
  online: boolean
  agent_version: string
  os: string
  simulator: boolean
  sleep_after: number // minutes until Windows sleeps: 0 never, -1 unknown
  needs_manual_update: boolean // stuck on a version that cannot update itself
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
  map: string
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
    // player_closed is the player closing Rust at the PC instead of tapping
    // cancel in the app. Same intent, same label.
    return job.reason_code === 'cancelled' || job.reason_code === 'player_closed'
      ? { label: 'Cancelled', tone: '' }
      : { label: 'Joined', tone: 'good' }
  }
  return { label: stateLabel(job.state), tone: 'warn' }
}
