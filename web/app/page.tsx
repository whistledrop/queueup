import { redirect } from 'next/navigation'
import { relayFetch, sessionToken } from '@/lib/relay'
import Dashboard from './dashboard'
import AppLanding from './appLanding'

export const dynamic = 'force-dynamic'

export default async function Home({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>
}) {
  // Signed in: straight to the app. Signed out: the front door.
  if (!(await sessionToken())) return <AppLanding />

  const res = await relayFetch('/api/auth/me')
  if (!res.ok) return <AppLanding />
  const me = (await res.json()) as { email: string; has_password?: boolean; has_login?: boolean }

  // Decided here on the server so the app never flashes up first and is then
  // taken away.
  // No way back in yet: no password, and no Google or Apple sign-in.
  if ((me.has_login ?? me.has_password) === false) {
    // An account from the old email-only signup that never set a password.
    // Everybody needs one now, free or paid, so there is one destination:
    // choose a password, then carry on.
    if ((await searchParams).subscribed === '1') redirect('/finish?subscribed=1')
    redirect('/finish')
  }

  return (
    <div className="shell">
      <Dashboard email={me.email} />
    </div>
  )
}
