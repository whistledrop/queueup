import { NextResponse } from 'next/server'
import { relayURL, SESSION_COOKIE, sessionCookieOptions, visitorHeaders } from '@/lib/relay'

export const dynamic = 'force-dynamic'

// Where "pick up where you left off" in a reminder email goes.
//
// One tap, wherever the mail app happens to open links, straight back to
// their own paywall: signed in as them, so the price and the countdown are
// their own, with the code applied on arrival. Done on the server and
// answered with a redirect, so there is no page in between to wait on.
//
// It can only do what typing the address on the landing page does. For an
// account that has paid or has a password it signs nobody in: they are sent
// to sign in, and the relay has already emailed somebody who paid without
// ever choosing a password the link to choose one.
export async function GET(request: Request) {
  const url = new URL(request.url)
  const promo = (url.searchParams.get('promo') ?? '').trim().toUpperCase().slice(0, 64)
  const to = (path: string) => {
    const u = new URL(path, url)
    if (promo) u.searchParams.set('promo', promo)
    return u
  }

  let upstream: Response
  try {
    upstream = await fetch(relayURL() + '/api/auth/continue', {
      method: 'POST',
      headers: { 'content-type': 'application/json', ...visitorHeaders(request.headers) },
      body: JSON.stringify({
        a: url.searchParams.get('a') ?? '',
        t: url.searchParams.get('t') ?? '',
        code: promo,
      }),
      cache: 'no-store',
    })
  } catch {
    // The relay is unreachable. The email box still works when it is back.
    return NextResponse.redirect(to('/'), 303)
  }

  const body = (await upstream.json().catch(() => ({}))) as { session_token?: string }
  if (upstream.status === 200 && body.session_token) {
    const res = NextResponse.redirect(to('/subscribe'), 303)
    res.cookies.set(SESSION_COOKIE, body.session_token, sessionCookieOptions)
    return res
  }
  if (upstream.status === 409 || upstream.status === 202) {
    return NextResponse.redirect(new URL('/login', url), 303)
  }
  // An old or broken link: the email box, with the code, gets them back.
  return NextResponse.redirect(to('/'), 303)
}
