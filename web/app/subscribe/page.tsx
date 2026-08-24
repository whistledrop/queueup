'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { api, getBilling } from '@/lib/api'
import { PLAN, priceLine } from '@/lib/pricing'

// The paywall. Someone lands here in exactly one situation: their PC is set up
// and they tapped Join without a subscription. Everything on this page assumes
// that moment: they are one step from the queue, not browsing.

export default function SubscribePage() {
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')

  // If they are already subscribed, or billing is off, this page has no job.
  useEffect(() => {
    getBilling()
      .then((b) => {
        if (b.subscribed) window.location.href = '/'
      })
      .catch(() => {})
  }, [])

  async function checkout() {
    setBusy(true)
    setNote('')
    try {
      // When Stripe is connected this returns its checkout page URL.
      const res = await api<{ url?: string }>('/api/billing/checkout', { method: 'POST' })
      if (res.url) window.location.href = res.url
    } catch (e) {
      setNote((e as Error).message)
      setBusy(false)
    }
  }

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn small" style={{ minHeight: 36, padding: '6px 12px' }}>
          Back
        </Link>
      </header>

      <div className="card">
        <h2>One thing left</h2>
        <p style={{ marginTop: 0, fontSize: 22, fontWeight: 700 }}>
          Your PC is set up. Joining is the paid part.
        </p>
        <p className="muted" style={{ marginTop: 0 }}>
          {priceLine()}, cancel anytime. Everything you have done so far stays
          free.
        </p>
        <ul style={{ margin: '0 0 4px', paddingLeft: 20, color: 'var(--muted)' }}>
          {PLAN.includes.map((line) => (
            <li key={line} style={{ margin: '6px 0' }}>{line}</li>
          ))}
        </ul>
      </div>

      {note && <div className="notice">{note}</div>}

      <button className="primary btn-wide" onClick={checkout} disabled={busy}>
        {busy ? 'One moment' : `Subscribe, ${priceLine()}`}
      </button>
      <p className="muted small" style={{ textAlign: 'center' }}>
        Payment is handled by Stripe. We never see your card.
      </p>
    </div>
  )
}
