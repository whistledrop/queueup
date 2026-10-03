'use client'

import { Suspense, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { api, getBilling, type Billing } from '@/lib/api'
import { PLAN, priceLine } from '@/lib/pricing'
import { capturePromoFromURL, storedPromo, storePromo } from '@/lib/promo'

// The paywall, and now the second screen of signing up rather than a gate
// somebody hits later.
//
// Nothing about the PC appears before this page. Most arrivals come from a
// video, on a phone, nowhere near their gaming PC: showing them a Windows
// download first is asking them to leave. So the order is account, price, pay,
// and only then the setup they need to be sitting at the PC for.
//
// Because they pay before trying it, the refund promise is load-bearing and is
// said plainly, not buried.

export default function SubscribePage() {
  return (
    <Suspense>
      <Subscribe />
    </Suspense>
  )
}

function Subscribe() {
  // ?preview=1 shows the page even while billing is off, so the whole
  // checkout can be tried in Stripe's test mode.
  const params = useSearchParams()
  const preview = params.get('preview') === '1'
  const [billing, setBilling] = useState<Billing | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')

  // The code, and what it is actually worth. Checked against Stripe rather
  // than assumed, so the number on this page is the number they get charged.
  const [code, setCode] = useState('')
  const [codeLine, setCodeLine] = useState('')
  const [firstMonth, setFirstMonth] = useState<number | null>(null)
  const [checking, setChecking] = useState(false)

  const check = useCallback(async (raw: string) => {
    const c = raw.trim().toUpperCase()
    if (!c) return
    setChecking(true)
    try {
      const res = await api<{ valid: boolean; line: string; first_month_pence?: number }>(
        '/api/billing/code',
        { method: 'POST', body: JSON.stringify({ code: c }) },
      )
      setCodeLine(res.line)
      setFirstMonth(res.valid ? (res.first_month_pence ?? null) : null)
      if (res.valid) storePromo(c)
    } catch (e) {
      setCodeLine((e as Error).message)
      setFirstMonth(null)
    } finally {
      setChecking(false)
    }
  }, [])

  useEffect(() => {
    capturePromoFromURL()
    const saved = storedPromo()
    if (saved) {
      setCode(saved)
      check(saved)
    }
    getBilling()
      .then((b) => {
        // Already paying, or nothing to pay for: this page has no job.
        if (b.paying || (b.subscribed && !preview)) window.location.href = '/'
        else setBilling(b)
      })
      .catch(() => {})
  }, [preview, check])

  async function checkout() {
    setBusy(true)
    setNote('')
    try {
      const res = await api<{ url?: string }>('/api/billing/checkout', {
        method: 'POST',
        body: JSON.stringify({ code: code.trim().toUpperCase() }),
      })
      if (res.url) window.location.href = res.url
    } catch (e) {
      setNote((e as Error).message)
      setBusy(false)
    }
  }

  const discounted = firstMonth !== null && firstMonth < PLAN.monthly * 100
  const nowPence = discounted ? firstMonth! : Math.round(PLAN.monthly * 100)

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/settings" className="btn quiet">Sign out</Link>
      </header>

      {billing?.test_mode && (
        <div className="notice">
          <b>Test mode.</b> No real money moves. Pay with card 4242 4242 4242
          4242, any future date, any three digits.
        </div>
      )}

      <div className="card" style={{ textAlign: 'center' }}>
        <p style={{ margin: '4px 0 0', fontSize: 15 }} className="muted">
          {discounted ? 'Your first month' : 'Every month'}
        </p>
        <p style={{ margin: '2px 0 0', fontSize: 52, fontWeight: 500, letterSpacing: '-0.04em' }}>
          {PLAN.symbol}
          {(nowPence / 100).toFixed(2)}
        </p>
        <p className="muted" style={{ margin: '0 0 18px' }}>
          {discounted ? `then ${priceLine()}. Cancel anytime.` : 'Cancel anytime.'}
        </p>
        <ul className="ticks">
          {PLAN.includes.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </div>

      <div className="card">
        <h2>Got a code?</h2>
        <form
          className="row"
          style={{ gap: 8, marginBottom: 0 }}
          onSubmit={(e) => {
            e.preventDefault()
            check(code)
          }}
        >
          <input
            value={code}
            onChange={(e) => setCode(e.target.value.toUpperCase())}
            placeholder="TIKTOK"
            maxLength={64}
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
            aria-label="Promo code"
            style={{ flex: 1 }}
          />
          <button type="submit" disabled={checking || !code.trim()} style={{ minHeight: 44 }}>
            {checking ? '...' : 'Apply'}
          </button>
        </form>
        {codeLine && (
          <p className={discounted ? 'small' : 'muted small'} style={{ margin: '10px 0 0' }}>
            {codeLine}
          </p>
        )}
      </div>

      {note && <div className="error">{note}</div>}

      <button className="primary btn-wide" onClick={checkout} disabled={busy} style={{ fontSize: 17 }}>
        {busy ? 'One moment' : `Subscribe for ${PLAN.symbol}${(nowPence / 100).toFixed(2)}`}
      </button>

      <div className="card" style={{ marginTop: 14 }}>
        <p style={{ margin: 0, fontWeight: 500 }}>
          Doesn&apos;t work on your setup? One-click refund, no questions.
        </p>
        <p className="muted small" style={{ margin: '6px 0 0' }}>
          QueueUp needs a Windows gaming PC you can leave switched on, with
          Steam and Rust installed. If that is not you, or it simply does not
          work, say so on the <Link href="/feedback">feedback page</Link> and
          you get your money back.
        </p>
      </div>

      <p className="muted small" style={{ textAlign: 'center', lineHeight: 1.5 }}>
        Payment by Stripe. We never see your card.
        <br />
        Cancel in two taps from Settings, any time. By subscribing you agree to
        the <Link href="/terms">terms</Link>.
      </p>
    </div>
  )
}
