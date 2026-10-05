// Everything that talks to the relay runs on this server, never in the browser.
//
// The session token lives in an http-only cookie, so no script on the page can
// read it, and the browser never holds a credential that could command someone's
// PC. Pages call /api/... on this app; this app calls the relay.

import { cookies, headers } from 'next/headers'

export const SESSION_COOKIE = 'queueup_session'

/**
 * The two headers that tell the relay which visitor this server is calling
 * for. Built from the request this server received, so the address is the
 * visitor's, not this server's.
 *
 * Without them the relay sees Netlify on every request, and the limits on
 * signing up and guessing passwords are counted against Netlify: five
 * signups an hour from the whole world, and then nobody. The key is what
 * makes the relay believe the address; it lives only on this server, and is
 * never sent to a browser.
 *
 * Only Netlify's own header is used. It is the one Netlify sets itself and
 * guarantees; anything a visitor could set for themselves is not an address
 * worth counting. With no key, or no address, nothing is sent and the relay
 * behaves exactly as it did before.
 */
export function visitorHeaders(incoming: Headers): Record<string, string> {
  const key = process.env.QUEUEUP_PROXY_KEY
  const ip = incoming.get('x-nf-client-connection-ip')?.trim()
  if (!key || !ip) return {}
  return { 'X-QueueUp-Proxy-Key': key, 'X-QueueUp-Visitor-IP': ip }
}

export function relayURL(): string {
  return process.env.RELAY_URL ?? 'http://127.0.0.1:8080'
}

export async function sessionToken(): Promise<string | null> {
  const store = await cookies()
  return store.get(SESSION_COOKIE)?.value ?? null
}

/** Call the relay as the signed-in user. Returns the raw response. */
export async function relayFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const token = await sessionToken()
  const out = new Headers(init.headers)
  for (const [k, v] of Object.entries(visitorHeaders(await headers()))) out.set(k, v)
  if (token) out.set('Authorization', `Bearer ${token}`)
  if (init.body && !out.has('content-type')) {
    out.set('content-type', 'application/json')
  }
  return fetch(relayURL() + path, { ...init, headers: out, cache: 'no-store' })
}

/** Call the relay and parse JSON, or return null if the call failed. */
export async function relayJSON<T>(path: string): Promise<T | null> {
  try {
    const res = await relayFetch(path)
    if (!res.ok) return null
    return (await res.json()) as T
  } catch {
    return null
  }
}

export const sessionCookieOptions = {
  httpOnly: true,
  sameSite: 'lax' as const,
  path: '/',
  // Secure in production. Left off locally so the app works over plain http.
  secure: process.env.NODE_ENV === 'production',
  maxAge: 60 * 60 * 24 * 30,
}
