import { redirect } from 'next/navigation'
import { relayFetch, sessionToken } from '@/lib/relay'
import Dashboard from './dashboard'

export const dynamic = 'force-dynamic'

export default async function Home() {
  // Checked on the server so a signed-out visitor never sees the page flash
  // before being sent to sign in.
  if (!(await sessionToken())) redirect('/login')

  const res = await relayFetch('/api/auth/me')
  if (!res.ok) redirect('/login')
  const me = (await res.json()) as { email: string }

  return <Dashboard email={me.email} />
}
