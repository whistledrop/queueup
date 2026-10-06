'use client'

// The first-month offer, on the landing page.
//
// Two pieces that have to agree with each other, and with the till.
//
// The discount exists only behind a promo code, so both ask the same question
// before they promise anything: does this visitor actually have one? For
// anybody arriving from a video that is yes, because the link carries the
// code and the TikTok check catches the links that do not. For somebody who
// typed the address in, it is no — and they are shown the price they will
// really pay, rather than a number that becomes a worse one at the till.

import { useEffect, useRef, useState } from 'react'
import { PLAN } from '@/lib/pricing'
import { storedPromo, storePromo } from '@/lib/promo'
import { track } from '@/lib/analytics'
import s from './landing.module.css'

const SEEN = 'queueup_offer_seen'
const money = (n: number) => `${PLAN.symbol}${n.toFixed(2)}`
// Worked out rather than written down, so it stays true if the price moves.
const PERCENT = Math.round((1 - PLAN.intro / PLAN.monthly) * 100)

/** The code this visitor holds, once the browser has had a chance to say. */
function useCode(): string | null {
  // null means "not asked yet": the server cannot know, and rendering a guess
  // would make the page flicker from one price to another.
  const [code, setCode] = useState<string | null>(null)
  useEffect(() => setCode(storedPromo()), [])
  return code
}

/**
 * The price on the Price card.
 *
 * It used to say "£1.99 first month" to everybody, including people with no
 * code, who then met £4.99 at the till. Being quoted one price and charged
 * another is the single worst thing a page can do to somebody at the moment
 * they decide to pay, so now only somebody who has the code is offered it.
 */
export function PriceAmount() {
  const code = useCode()
  if (code) {
    return (
      <>
        {money(PLAN.intro)}
        <small>
          {' '}
          first month, then {money(PLAN.monthly)}
        </small>
      </>
    )
  }
  return (
    <>
      {money(PLAN.monthly)}
      <small> a month</small>
    </>
  )
}

/** The line under the Price card's button. */
export function PriceNote() {
  const code = useCode()
  return (
    <>
      {code
        ? `Your ${money(PLAN.intro)} first month is applied at checkout. After that ${money(PLAN.monthly)} a month, cancelled in two taps, any time, and you keep the days you have paid for.`
        : `${money(PLAN.monthly)} a month. Cancel in two taps, any time, and keep the days you have paid for.`}
    </>
  )
}

/**
 * The offer, as a card that appears a few seconds in.
 *
 * Somebody who arrived from a video already has the discount, and no way of
 * knowing: the price is four sections down a page most of them never scroll.
 * This says it out loud while they are still reading, and the button is one
 * tap with nothing to type.
 *
 * What the button does is real — it writes the code down, which is what the
 * till reads — but for most people it was already written when they arrived.
 * So the card is careful to claim only what is true: the discount is theirs
 * and it comes off at checkout. It never invents a reason they might lose it.
 */
export default function OfferPopup() {
  const code = useCode()
  const [show, setShow] = useState(false)
  const [claimed, setClaimed] = useState(false)
  const card = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!code) return
    try {
      if (localStorage.getItem(SEEN)) return
    } catch {
      // Storage blocked: it would show on every page load, which is worse
      // than never showing. Stay quiet.
      return
    }
    const t = setTimeout(() => {
      // Never interrupt somebody already typing their address. They are
      // doing the thing this card exists to ask for.
      const email = document.querySelector<HTMLInputElement>('input[type="email"]')
      if (email && (email.value.trim() !== '' || document.activeElement === email)) return
      setShow(true)
      track('offer_shown', { code })
    }, 5000)
    return () => clearTimeout(t)
  }, [code])

  // Remember it either way, the moment it is seen: claimed or waved away,
  // nobody gets it twice.
  function remember() {
    try {
      localStorage.setItem(SEEN, '1')
    } catch {
      // as above
    }
  }

  function close() {
    remember()
    setShow(false)
  }

  function claim() {
    if (code) storePromo(code)
    remember()
    setClaimed(true)
    track('offer_claimed', { code: code ?? '' })
  }

  // Closing with the keyboard, and the focus moved onto the card, so somebody
  // on a keyboard or a screen reader is not left behind on the page below.
  useEffect(() => {
    if (!show) return
    card.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [show])

  /** Out of the way, and on to the one thing the page is asking for. */
  function done() {
    setShow(false)
    const email = document.querySelector<HTMLInputElement>('input[type="email"]')
    email?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    email?.focus({ preventScroll: true })
  }

  if (!show || !code) return null

  return (
    <div className={s.offerWrap} onClick={close}>
      <div
        className={s.offerCard}
        role="dialog"
        aria-modal="true"
        aria-labelledby="offerTitle"
        tabIndex={-1}
        ref={card}
        onClick={(e) => e.stopPropagation()}
      >
        <button className={s.offerClose} onClick={close} aria-label="Close">
          ×
        </button>

        {claimed ? (
          <>
            <p className={s.offerTick} aria-hidden="true">
              ✓
            </p>
            <h2 id="offerTitle" className={s.offerTitle}>
              Claimed
            </h2>
            <p className={s.offerBody}>
              Your {money(PLAN.intro)} first month is applied at checkout. Enter your email
              to set it up — it takes about two minutes.
            </p>
            <button className={s.offerCta} onClick={done}>
              Get started
            </button>
          </>
        ) : (
          <>
            <p className={s.offerKicker}>Your code: {code}</p>
            <h2 id="offerTitle" className={s.offerTitle}>
              {PERCENT}% off your first month
            </h2>
            <p className={s.offerBody}>
              <b>
                {money(PLAN.intro)} <span className={s.offerWas}>{money(PLAN.monthly)}</span>
              </b>{' '}
              for your first month, then {money(PLAN.monthly)} a month. Cancel anytime.
            </p>
            <button className={s.offerCta} onClick={claim}>
              Claim {PERCENT}% off
            </button>
            <button className={s.offerNo} onClick={close}>
              No thanks
            </button>
          </>
        )}
      </div>
    </div>
  )
}
