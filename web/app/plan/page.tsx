'use client'

// The plan choice, straight after signing up.
//
// This screen exists because QueueUp is no longer paid-only. The old flow sent
// everybody from signup to the price, which is the right screen only if paying
// is the single way in; now it is one of two, and sending a free player to a
// paywall they do not need is how a free plan quietly fails.
//
// Free is on the left and chosen first on purpose. It is the honest default
// for somebody who has not seen QueueUp work yet, and the paid plan reads
// better next to it than it does alone.

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { track } from '@/lib/analytics'
import { PLAN, introLine } from '@/lib/pricing'
import { FREE_JOINS_PER_MONTH } from '@/lib/plan'
import s from './plan.module.css'

export default function PlanPage() {
  return (
    <Suspense>
      <ChoosePlan />
    </Suspense>
  )
}

function ChoosePlan() {
  const router = useRouter()
  const [busy, setBusy] = useState<'free' | 'paid' | ''>('')

  // Free needs nothing bought and nothing recorded: an account with no
  // subscription IS the free plan. The one thing still missing is a password,
  // which was skipped on the landing page to keep that form to a single
  // field, so it is collected here on the way to the PC setup.
  function chooseFree() {
    if (busy) return
    setBusy('free')
    track('plan_chosen', { plan: 'free' })
    router.push('/finish?next=get')
  }

  function choosePaid() {
    if (busy) return
    setBusy('paid')
    track('plan_chosen', { plan: 'paid' })
    router.push('/subscribe')
  }

  return (
    <div className="shell narrow">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
      </header>

      <h1 className={s.heading}>You&rsquo;re in. Pick how you want to use it.</h1>
      <p className={s.sub}>You can change this later, either way.</p>

      <div className={s.plans}>
        <button type="button" className={s.plan} onClick={chooseFree} disabled={!!busy}>
          <span className={s.name}>Free</span>
          <span className={s.price}>{PLAN.symbol}0</span>
          <span className={s.after}>No card needed</span>
          <ul className={s.list}>
            <li>{FREE_JOINS_PER_MONTH} joins a month</li>
            <li>One wipe day join, to try it</li>
            <li>Live status on your phone</li>
            <li>Rejoins by itself if Rust crashes</li>
          </ul>
          <span className={s.cta}>{busy === 'free' ? 'Setting up…' : 'Start free'}</span>
        </button>

        <button
          type="button"
          className={`${s.plan} ${s.planPaid}`}
          onClick={choosePaid}
          disabled={!!busy}
        >
          <span className={s.name}>Unlimited</span>
          <span className={s.price}>{PLAN.trialDays} days free</span>
          <span className={s.after}>then {PLAN.symbol}{PLAN.monthly.toFixed(2)} a month</span>
          <ul className={s.list}>
            <li>Unlimited joins, every wipe</li>
            <li>Schedule any join, any time</li>
            <li>Auto-join the moment a wipe ends</li>
          </ul>
          <span className={s.cta}>{busy === 'paid' ? 'Opening…' : 'Start free trial'}</span>
        </button>
      </div>

      <p className={s.footnote}>
        {introLine()}. Cancel anytime, in two taps.
      </p>
    </div>
  )
}
