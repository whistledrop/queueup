import { redirect } from 'next/navigation'
import { relayFetch, sessionToken } from '@/lib/relay'
import Dashboard from './dashboard'
import Landing from './landing'

export const dynamic = 'force-dynamic'

export default async function Home({
  searchParams,
}: {
  searchParams: Promise<{ [key: string]: string | string[] | undefined }>
}) {
  // Signed in: straight to the app. Signed out: the front door.
  if (!(await sessionToken())) return <Landing />

  const res = await relayFetch('/api/auth/me')
  if (!res.ok) return <Landing />
  const me = (await res.json()) as { email: string; has_password?: boolean }

  // Signed up with only an email. Not paid yet: the price, which is the only
  // thing there is for them. Paid but closed the tab before choosing a
  // password: that step, before anything else, because until they do this
  // browser is the only place they could ever get back in. Decided here on
  // the server so the app never flashes up first and is then taken away.
  if (me.has_password === false) {
    // Straight back from Stripe. The payment may still be on its way to us,
    // so asking billing now could say "not paid" to somebody who just has:
    // they go to the password step, and it knows not to send them back.
    if ((await searchParams).subscribed === '1') redirect('/finish?subscribed=1')
    const bill = await relayFetch('/api/billing')
    const billing = bill.ok ? ((await bill.json()) as { enabled?: boolean; subscribed?: boolean }) : null
    redirect(billing?.enabled && !billing.subscribed ? '/subscribe' : '/finish')
  }

  return (
    <div className="shell">
      <Dashboard email={me.email} />
    </div>
  )
}
