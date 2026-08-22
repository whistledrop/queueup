import { cookies } from 'next/headers'
import { relayURL, SESSION_COOKIE, sessionCookieOptions } from '@/lib/relay'

export const dynamic = 'force-dynamic'

export async function POST(request: Request) {
  return signIn(request, '/api/auth/login')
}

// Shared by sign in and create account: both come back with a session token,
// which is put straight into an http-only cookie and never handed to the page.
export async function signIn(request: Request, relayPath: string) {
  let upstream: Response
  try {
    upstream = await fetch(relayURL() + relayPath, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: await request.text(),
      cache: 'no-store',
    })
  } catch {
    return Response.json(
      { error: "We can't reach QueueUp right now. Try again in a moment." },
      { status: 503 },
    )
  }

  const body = await upstream.json().catch(() => ({}))
  if (!upstream.ok) {
    return Response.json(
      { error: body.error ?? 'That did not work. Try again.' },
      { status: upstream.status },
    )
  }

  const store = await cookies()
  store.set(SESSION_COOKIE, body.session_token, sessionCookieOptions)
  return Response.json({ account: body.account })
}
