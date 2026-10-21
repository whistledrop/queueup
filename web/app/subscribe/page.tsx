'use client'

import { Suspense, useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { api, ApiError, getBilling, type Billing } from '@/lib/api'
import { PLAN, priceLine } from '@/lib/pricing'
import { FREE_JOINS_PER_MONTH } from '@/lib/plan'
import { capturePromoFromURL, claimCode, storedPromo, storePromo } from '@/lib/promo'
import s from './subscribe.module.css'
import { track } from '@/lib/analytics'

// The paywall, and the second screen of signing up rather than a gate somebody
// hits later.
//
// Nothing about the PC appears before this page. Most arrivals come from a
// video, on a phone, nowhere near their gaming PC: showing them a Windows
// download first is asking them to leave. So the order is account, price, pay,
// and only then the setup they need to be sitting at the PC for.
//
// Which also means they are paying for something they have not seen work.
// What carries that is how plainly it can be cancelled.

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
  // Why they are here. Set by lib/api.ts when the relay refuses something,
  // so the page opens on the thing they just tried to do rather than on a
  // generic price. Anything unrecognised is treated as no reason at all.
  const reason = params.get('reason')
  const gateLine =
    reason === 'free_joins_used'
      ? "You've used all 10 free joins this month."
      : reason === 'free_wipe_join_used'
        ? "You've already used your one free wipe day join."
        : ''
  const [billing, setBilling] = useState<Billing | null>(null)
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState('')

  // The code they arrived with. It no longer changes the price; it is checked
  // and kept because it says where they came from.
  const [code, setCode] = useState('')
  const [applied, setApplied] = useState('')
  // A real code whose offer has run out. Kept apart from "invalid", because
  // it still counts for attribution and the page should say what happened
  // rather than just show a higher price.
  // Nothing about the price is shown until billing has said whether this
  // account gets the free trial, so the page never changes its mind mid-read.
  const [codeKnown, setCodeKnown] = useState(false)
  const [billingKnown, setBillingKnown] = useState(false)

  // Checks a code that arrived in a link and keeps it if real. Silent either
  // way: there is no box to complain in, and it changes no price.
  const check = useCallback(async (raw: string) => {
    const c = raw.trim().toUpperCase()
    if (!c) return
    try {
      const res = await api<{ valid: boolean }>('/api/billing/code', {
        method: 'POST',
        body: JSON.stringify({ code: c }),
      })
      setApplied(res.valid ? c : '')
      if (res.valid) storePromo(c)
    } catch {
      setApplied('')
    }
  }, [])

  useEffect(() => {
    capturePromoFromURL()
    // The code they arrived on, theirs or the site's own, for attribution.
    const saved = claimCode()
    if (saved) {
      setCode(saved)
      check(saved).finally(() => setCodeKnown(true))
    } else {
      setCodeKnown(true)
    }
    getBilling()
      .then((b) => {
        // Already paying, or nothing to pay for: this page has no job.
        if (b.paying || (b.subscribed && !preview)) window.location.href = '/'
        else {
          setBilling(b)
          setBillingKnown(true)
        }
      })
      .catch((e) => {
        // Signed out. Usually somebody who signed up inside TikTok's browser
        // and tapped a reminder email that opened somewhere else. The way
        // back is the email box on the front page, and the code they came
        // on goes with them so the price they see is the one in the email.
        if (e instanceof ApiError && e.status === 401) {
          const code = new URLSearchParams(window.location.search).get('promo') ?? storedPromo()
          window.location.href = code ? `/?promo=${encodeURIComponent(code)}` : '/'
          return
        }
        // Anything else: show what we have rather than nothing at all.
        setBillingKnown(true)
      })
  }, [preview, check])

  // Signing out from the paywall used to be a link to Settings, which does
  // not sign anybody out: it dropped them into the app, full navigation and
  // all, which is the one place somebody who has not paid should not be.
  //
  // A hard navigation rather than router.push, because signing out has to
  // drop every page the client router is still holding. The landing page is
  // where they came in, and where they sign in again from.
  async function signOut() {
    try {
      await fetch('/api/auth/logout', { method: 'POST' })
    } catch {
      // The cookie is dropped server-side either way; go anyway.
    }
    window.location.href = '/'
  }

  async function checkout() {
    setBusy(true)
    setNote('')
    // Fired before leaving for Stripe, not after coming back: this is the
    // count of people who got as far as wanting to pay, and the gap between
    // it and subscription_paid is the cost of the payment page itself.
    track('checkout_started', applied ? { promo_code: applied } : undefined, { leaving: true })
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
  // No discounts since 2026-10-21: the price is the price, and a first
  // subscription starts with a free trial instead.
  const trial = billing?.trial_days ?? 0
  const priceReady = codeKnown && billingKnown

  // Counted once the price is settled, so the event carries what they were
  // actually shown rather than what the page looked like mid-load.
  const counted = useRef(false)
  useEffect(() => {
    if (!priceReady || counted.current) return
    counted.current = true
    track('paywall_viewed', {
      price_pence: nowPence,
      trial_days: trial,
      ...(applied ? { promo_code: applied } : {}),
    })
    if (reason) {
      track('upgrade_prompt_shown', { reason })
      if (reason === 'free_joins_used') track('free_limit_reached')
      if (reason === 'free_wipe_join_used') track('free_wipe_join_used')
    }
    // Only when the page settles; the values are read at that moment.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [priceReady])
  const nowPence = full
  const money = (pence: number) => `${PLAN.symbol}${(pence / 100).toFixed(2)}`

  // Free is only worth offering to somebody who is actually on it.
  const onFree = billing?.enabled === true && billing.plan === 'free'
  // "back on the 1st" beats a date: it is the rule, not one month's version
  // of it, and it is the same sentence every month.
  const refills = billing?.joins_left
    ? `${billing.joins_left} left right now`
    : 'back on the 1st'

  return (
    <div className="shell narrow">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <button type="button" className="tab" onClick={signOut}>
          Sign out
        </button>
      </header>

      {billing?.test_mode && (
        <div className="notice">
          <b>Test mode.</b> No real money moves. Pay with card 4242 4242 4242
          4242, any future date, any three digits.
        </div>
      )}

      {note && <div className="error">{note}</div>}

      {gateLine && (
        <div className={s.gate}>
          <b>{gateLine}</b> They come back on the 1st. Unlimited joins start
          here.
        </div>
      )}

      <div className={s.panel}>
        <div className={priceReady ? undefined : s.pending} aria-busy={!priceReady}>
          <p className={s.kicker}>{trial > 0 ? 'QueueUp Unlimited' : 'QueueUp'}</p>
          <div className={s.priceRow}>
            <span className={s.price}>{trial > 0 ? `${trial} days free` : money(nowPence)}</span>
          </div>
          <p className={s.after}>
            {trial > 0
              ? `then ${priceLine()}. Cancel before the trial ends and you pay nothing.`
              : 'a month. Cancel anytime.'}
          </p>
        </div>

        <ul className={s.features}>
          {PLAN.includes.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>

        <button className={s.cta} onClick={checkout} disabled={busy || !priceReady}>
          {busy || !priceReady
            ? 'One moment'
            : trial > 0
              ? 'Start my free trial'
              : 'Subscribe'}
        </button>

        <p className={s.afterPay}>Next: 2-minute PC setup</p>

        {/* The free plan, on the paywall itself.
            Somebody sent here by the gate has already used what they were
            given, and a page with one button and no way out reads as a wall
            rather than an offer. Saying what free still gets them costs a few
            upgrades now and keeps the account, which is worth more than a
            paywall nobody can leave. */}
        {onFree && (
          <div className={s.free}>
            <p className={s.freeHead}>
              Or stay on Free: {billing?.free_joins ?? FREE_JOINS_PER_MONTH} joins a
              month, {refills}
            </p>
            <Link href="/" className={s.freeLink} onClick={() => track('plan_chosen', { plan: 'free', from: 'paywall' })}>
              Keep using the free plan
            </Link>
          </div>
        )}

        {/* The two things that stop people paying for a Rust tool, met at the
            moment they decide. No refund promise: cancelling keeps the days
            already paid for, which is the thing that is actually true. */}
        <p className={s.assure}>
          <span aria-hidden="true">🛡</span> Not a cheat. Never touches the game.
          Can&apos;t get you banned.
        </p>
        <p className={s.assure}>
          <span aria-hidden="true">↩</span> Cancel in two taps, any time, and keep the days
          you have paid for.
        </p>

        <p className={s.trust}>
          <Lock /> Secure payment by Stripe
        </p>

        <blockquote className={s.quote}>
          <p>
            “I&apos;m usually at school for when servers wipe. With this app I was able to
            join queue whilst in class, and by the time I was home my PC was in the
            server.”
          </p>
          <cite>— 9k-hour Rust player, founder</cite>
        </blockquote>

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

