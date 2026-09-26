import { cookies } from 'next/headers'
import { relayURL, SESSION_COOKIE, sessionToken, sessionCookieOptions } from '@/lib/relay'

export const dynamic = 'force-dynamic'

// Changing a password ends every session, including this browser's, so this
// cannot go through the ordinary relay proxy: the cookie has to be swapped for
// the replacement token in the same breath. Otherwise the person who just
// changed their password would be the one thrown out.
export async function POST(request: Request) {
  const token = await sessionToken()
  let upstream: Response
  try {
    upstream = await fetch(relayURL() + '/api/auth/password', {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
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
  return Response.json({ status: body.status })
}
