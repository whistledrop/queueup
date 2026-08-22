// Everything that talks to the relay runs on this server, never in the browser.
//
// The session token lives in an http-only cookie, so no script on the page can
// read it, and the browser never holds a credential that could command someone's
// PC. Pages call /api/... on this app; this app calls the relay.

import { cookies } from 'next/headers'

export const SESSION_COOKIE = 'queueup_session'

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
  const headers = new Headers(init.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  if (init.body && !headers.has('content-type')) {
    headers.set('content-type', 'application/json')
  }
  return fetch(relayURL() + path, { ...init, headers, cache: 'no-store' })
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
