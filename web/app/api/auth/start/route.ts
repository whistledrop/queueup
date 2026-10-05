import { cookies } from 'next/headers'
import { relayURL, SESSION_COOKIE, sessionCookieOptions, visitorHeaders } from '@/lib/relay'

export const dynamic = 'force-dynamic'

// Starting an account from an email address alone.
//
// Unlike sign-in, this can answer three ways, and only one of them comes with
// a session: a new account, or somebody back to an empty one they already
// started. The other two — "you already have an account, sign in" and "we've
// emailed you a link" — carry no token, and the page is told what happened
// without ever seeing one either way.
export async function POST(request: Request) {
  let upstream: Response
  try {
    upstream = await fetch(relayURL() + '/api/auth/start', {
      method: 'POST',
      headers: { 'content-type': 'application/json', ...visitorHeaders(request.headers) },
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
  const { session_token: token, ...rest } = body as { session_token?: string } & Record<string, unknown>
  if (upstream.ok && token) {
    const store = await cookies()
    store.set(SESSION_COOKIE, token, sessionCookieOptions)
  }
  return Response.json(rest, { status: upstream.status })
}
