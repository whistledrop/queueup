import { cookies } from 'next/headers'
import { relayFetch, SESSION_COOKIE } from '@/lib/relay'

export const dynamic = 'force-dynamic'

export async function POST() {
  // Tell the relay to forget the session, then drop the cookie. Even if the
  // relay call fails, the cookie goes, so signing out always signs you out.
  try {
    await relayFetch('/api/auth/logout', { method: 'POST' })
  } catch {
    // nothing to do
  }
  const store = await cookies()
  store.delete(SESSION_COOKIE)
  return Response.json({ status: 'signed out' })
}
