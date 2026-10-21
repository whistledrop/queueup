import { cookies } from 'next/headers'
import { relayURL, SESSION_COOKIE, sessionCookieOptions, visitorHeaders } from '@/lib/relay'

export const dynamic = 'force-dynamic'

// "Continue with Google" and "Continue with Apple". The browser did the
// sign-in with the provider; this hands the provider's ID token to the relay,
// which checks it, and keeps the session it answers with in an http-only
// cookie, exactly as email sign-in does.
export async function POST(request: Request) {
  let upstream: Response
  try {
    upstream = await fetch(relayURL() + '/api/auth/oauth', {
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
