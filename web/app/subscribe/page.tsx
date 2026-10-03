'use client'

import { Suspense, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { api, getBilling, type Billing } from '@/lib/api'
import { PLAN, priceLine } from '@/lib/pricing'
import { capturePromoFromURL, storedPromo, storePromo } from '@/lib/promo'
import s from './subscribe.module.css'

// The paywall, and the second screen of signing up rather than a gate somebody
// hits later.
//
// Nothing about the PC appears before this page. Most arrivals come from a
// video, on a phone, nowhere near their gaming PC: showing them a Windows
// download first is asking them to leave. So the order is account, price, pay,
// and only then the setup they need to be sitting at the PC for.
//
// Which also means they are paying for something they have not seen work. Two
// things carry that: the three steps that follow, so paying does not feel like
// the edge of a cliff, and how plainly it can be cancelled.

export default function SubscribePage() {
  return (
    <Suspense>
      <Subscribe />
    </Suspense>
  )
}

function Subscribe() {
  // ?preview=1 shows the page even while billing is off, so the whole checkout
  // can be tried without switching the gate on for everybody.
  const params = useSearchParams()
  const preview = params.get('preview') === '1'
  const [billing, setBilling] = useState<Billing | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')

  // The code, and what it is actually worth. Checked against Stripe rather
  // than assumed, so the number on this page is the number they get charged.
  const [code, setCode] = useState('')
  const [codeOpen, setCodeOpen] = useState(false)
  const [codeBad, setCodeBad] = useState('')
  const [applied, setApplied] = useState('')
  const [firstMonth, setFirstMonth] = useState<number | null>(null)
  const [checking, setChecking] = useState(false)

  // quiet is for the code we found saved from a ?promo= link rather than one
  // they typed. If that one fails there is nothing for them to fix and nothing
  // they asked for, so complaining at somebody who has not touched the box is
  // just noise on the screen where they are deciding whether to pay.
  const check = useCallback(async (raw: string, quiet = false) => {
    const c = raw.trim().toUpperCase()
    if (!c) return
    setChecking(true)
    setCodeBad('')
    try {
      const res = await api<{ valid: boolean; line: string; first_month_pence?: number }>(
        '/api/billing/code',
        { method: 'POST', body: JSON.stringify({ code: c }) },
      )
      if (res.valid) {
        setApplied(c)
        setFirstMonth(res.first_month_pence ?? null)
        setCodeOpen(false)
        storePromo(c)
      } else {
        setApplied('')
        setFirstMonth(null)
        if (!quiet) setCodeBad(res.line)
      }
    } catch (e) {
      setApplied('')
      setFirstMonth(null)
      if (!quiet) setCodeBad((e as Error).message)
    } finally {
      setChecking(false)
    }
  }, [])

  useEffect(() => {
    capturePromoFromURL()
    const saved = storedPromo()
    if (saved) {
      setCode(saved)
      check(saved, true)
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
        body: JSON.stringify({ code: applied }),
      })
      if (res.url) window.location.href = res.url
    } catch (e) {
      setNote((e as Error).message)
      setBusy(false)
    }
  }

  const full = Math.round(PLAN.monthly * 100)
  const discounted = firstMonth !== null && firstMonth < full
  const nowPence = discounted ? firstMonth! : full
  const money = (pence: number) => `${PLAN.symbol}${(pence / 100).toFixed(2)}`

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/settings" className="tab">Sign out</Link>
      </header>

      {billing?.test_mode && (
        <div className="notice">
          <b>Test mode.</b> No real money moves. Pay with card 4242 4242 4242
          4242, any future date, any three digits.
        </div>
      )}

      {note && <div className="error">{note}</div>}

      <div className={s.panel}>
        <p className={s.headline}>Wipe day, two ways</p>

        {/* Three beats each, the same three beats, so the difference is the
            only thing that moves. Anything longer gets skimmed: this is read
            on a phone, by somebody deciding in about four seconds. */}
        <div className={s.compare}>
          <div className={s.was_}>
            <p className={s.compareLabel}>Without QueueUp</p>
            <p className={s.compareBody}>
              Home at eight.<br />
              212 in the queue.<br />
              Playing at half ten.
            </p>
          </div>
          <div className={s.now_}>
            <p className={s.compareLabel}>With QueueUp</p>
            <p className={s.compareBody}>
              Tap join at two.<br />
              Walk in at eight.<br />
              Already in.
            </p>
          </div>
        </div>

        <div className={s.priceBlock}>
          <p className={s.kicker}>{discounted ? 'Your first month' : 'QueueUp'}</p>
          <div className={s.priceRow}>
            {discounted && <span className={s.was}>{money(full)}</span>}
            <span className={s.price}>{money(nowPence)}</span>
          </div>
          <p className={s.after}>
            {discounted ? `then ${priceLine()}. Cancel anytime.` : 'a month. Cancel anytime.'}
          </p>

          {applied && discounted && (
            <p className={s.codeApplied}>
              <Tick /> Code {applied} applied
            </p>
          )}

          <button className={s.cta} onClick={checkout} disabled={busy}>
            {busy ? 'One moment' : `Subscribe for ${money(nowPence)}`}
          </button>

          <p className={s.trust}>
            <Lock /> Secure payment by Stripe
          </p>

          {!applied && !codeOpen && (
            <button className={s.codeToggle} onClick={() => setCodeOpen(true)}>
              Have a code?
            </button>
          )}

          {codeOpen && (
            <form
              className={s.codeRow}
              onSubmit={(e) => {
                e.preventDefault()
                check(code)
              }}
            >
              <input
                value={code}
                onChange={(e) => setCode(e.target.value.toUpperCase())}
                placeholder="CODE"
                maxLength={64}
                autoFocus
                // The box may already hold a code that arrived in the link and
                // failed. Typing over it should replace it, not append to it.
                onFocus={(e) => e.currentTarget.select()}
                autoCapitalize="characters"
                autoCorrect="off"
                spellCheck={false}
                aria-label="Promo code"
              />
              <button type="submit" disabled={checking || !code.trim()}>
                {checking ? '...' : 'Apply'}
              </button>
            </form>
          )}
          {codeBad && <p className={s.codeBad}>{codeBad}</p>}
        </div>
      </div>

      <div className={s.included}>
        <h3>What you get</h3>
        <ul className={s.features}>
          {PLAN.includes.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </div>

      <div className={s.next}>
        <h3>What happens next</h3>
        <ol className={s.steps}>
          <li>
            <b>Link your PC.</b> One file, one six character code. About two
            minutes, and only ever once.
          </li>
          <li>
            <b>Save the servers you play.</b> So wipe day is one tap, not a
            search.
          </li>
          <li>
            <b>Join from anywhere.</b> Your PC queues while you are at work, at
            school, or on the bus.
          </li>
        </ol>
      </div>

      <div className={s.refund}>
        <p>Cancel whenever you like, in two taps.</p>
        <p>
          It is one button in Settings, you keep the days you have paid for,
          and nothing is taken after that. QueueUp needs a Windows gaming PC
          you can leave switched on, with Steam and Rust installed, so check
          that is you before you start.
        </p>
      </div>

      <p className={s.smallprint}>
        We never see your card. By subscribing you agree to the{' '}
        <Link href="/terms">terms</Link>.
      </p>
    </div>
  )
}

function Lock() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <rect x="4" y="10" width="16" height="11" rx="2.5" stroke="currentColor" strokeWidth="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3" stroke="currentColor" strokeWidth="2" />
    </svg>
  )
}

function Tick() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path d="M4 12.5l5.5 5.5L20 7" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
