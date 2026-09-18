'use client'

import { Suspense, useEffect, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { api, getBilling, type Billing } from '@/lib/api'
import { PLAN, priceLine } from '@/lib/pricing'

// The paywall. Someone lands here in exactly one situation: their PC is set up
// and they tapped Join without a subscription. Everything on this page assumes
// that moment: they are one step from the queue, not browsing.
//
// The price is said in full, both halves, before they pay: £1.99 now AND £4.99
// after. A first-month offer that only mentions the first month is how people
// end up feeling tricked, and feeling tricked is a chargeback.

export default function SubscribePage() {
  return (
    <Suspense>
      <Subscribe />
    </Suspense>
  )
}

function Subscribe() {
  // ?preview=1 shows the page even while billing is off, so the whole
  // checkout can be tried in Stripe's test mode during the free beta.
  const preview = useSearchParams().get('preview') === '1'
  const [billing, setBilling] = useState<Billing | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')

  useEffect(() => {
    getBilling()
      .then((b) => {
        // Already paying, or nothing to pay for: this page has no job.
        if (b.paying || (b.subscribed && !preview)) window.location.href = '/'
        else setBilling(b)
      })
      .catch(() => {})
  }, [preview])

  async function checkout() {
    setBusy(true)
    setNote('')
    try {
      const res = await api<{ url?: string }>('/api/billing/checkout', { method: 'POST' })
      if (res.url) window.location.href = res.url
    } catch (e) {
      setNote((e as Error).message)
      setBusy(false)
    }
  }

  const intro = billing?.intro_available ?? true
  const now = intro ? PLAN.intro : PLAN.monthly

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn quiet">Back</Link>
      </header>

      {billing?.test_mode && (
        <div className="notice">
          <b>Test mode.</b> No real money moves. Pay with card 4242 4242 4242
          4242, any future date, any three digits.
        </div>
      )}

      <div className="card" style={{ textAlign: 'center' }}>
        <h2>Your PC is ready</h2>
        <p style={{ margin: '4px 0 0', fontSize: 15 }} className="muted">
          {intro ? 'Your first month' : 'Every month'}
        </p>
        <p style={{ margin: '2px 0 0', fontSize: 52, fontWeight: 800, letterSpacing: '-0.03em' }}>
          {PLAN.symbol}
          {now.toFixed(2)}
        </p>
        <p className="muted" style={{ margin: '0 0 18px' }}>
          {intro ? `then ${priceLine()}. Cancel anytime.` : 'Cancel anytime.'}
        </p>
        <ul className="ticks">
          {PLAN.includes.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </div>

      {note && <div className="notice">{note}</div>}

      <button className="primary btn-wide" onClick={checkout} disabled={busy} style={{ fontSize: 17 }}>
        {busy ? 'One moment' : intro ? `Start for ${PLAN.symbol}${PLAN.intro.toFixed(2)}` : `Subscribe, ${priceLine()}`}
      </button>
      <p className="muted small" style={{ textAlign: 'center', lineHeight: 1.5 }}>
        Payment by Stripe. We never see your card.
        <br />
        Cancel in two taps from your dashboard, any time. By subscribing you
        agree to the <Link href="/terms">terms</Link>.
      </p>
    </div>
  )
}
