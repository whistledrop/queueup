import { relayFetch, sessionToken } from '@/lib/relay'
import Dashboard from './dashboard'
import Landing from './landing'

export const dynamic = 'force-dynamic'

export default async function Home() {
  // Signed in: straight to the app. Signed out: the front door.
  if (!(await sessionToken())) return <Landing />

  const res = await relayFetch('/api/auth/me')
  if (!res.ok) return <Landing />
  const me = (await res.json()) as { email: string }

  return (
    <div className="shell">
      <Dashboard email={me.email} />
    </div>
  )
}
