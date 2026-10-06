'use client'

// The "Get QueueUp" buttons further down the page, and the box they open.
//
// The hero has the email box, but somebody who has just read How it works or
// Fair play is a long scroll from it. Pressing one of these opens the very
// same box — the offer above it, the address, Claim — over wherever they are,
// so acting never means finding their way back up the page.

import { useEffect, useRef, useState } from 'react'
import { track } from '@/lib/analytics'
import StartForm from './startForm'
import s from './landing.module.css'

/** The claim box, over the page. */
function ClaimModal({ where, onClose }: { where: string; onClose: () => void }) {
  const card = useRef<HTMLDivElement>(null)
  // Held in a ref so the effect below runs once, on opening, rather than
  // again every time the button that opened it re-renders.
  const close = useRef(onClose)
  close.current = onClose

  useEffect(() => {
    // The cursor straight into the address field: the box exists to be typed in.
    card.current?.querySelector<HTMLInputElement>('input[type="email"]')?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close.current()
    }
    window.addEventListener('keydown', onKey)
    // The page underneath stays where it is while the box is open.
    const was = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      window.removeEventListener('keydown', onKey)
      document.body.style.overflow = was
    }
  }, [])

  return (
    <div className={s.offerWrap} onClick={onClose}>
      <div
        className={s.offerCard}
        role="dialog"
        aria-modal="true"
        aria-label="Get QueueUp"
        ref={card}
        onClick={(e) => e.stopPropagation()}
      >
        <button className={s.offerClose} onClick={onClose} aria-label="Close">
          ×
        </button>
        <StartForm className={s.startForm} from={where} />
      </div>
    </div>
  )
}

/** A "Get QueueUp" button that opens the claim box, and says where it was. */
export function CtaRow({ where }: { where: string }) {
  const [open, setOpen] = useState(false)
  return (
    <div className={s.ctaRow}>
      <button
        type="button"
        className={s.cta}
        onClick={() => {
          track('cta_clicked', { position: where })
          setOpen(true)
        }}
      >
        Get QueueUp
      </button>
      {open && <ClaimModal where={where} onClose={() => setOpen(false)} />}
    </div>
  )
}

/** The same, as a bar along the bottom of a phone. Hidden on wider screens. */
export function StickyCta() {
  const [open, setOpen] = useState(false)
  return (
    <>
      <div className={s.sticky}>
        <button
          type="button"
          className={s.stickyBtn}
          onClick={() => {
            track('cta_clicked', { position: 'sticky' })
            setOpen(true)
          }}
        >
          Get QueueUp
        </button>
      </div>
      {open && <ClaimModal where="sticky" onClose={() => setOpen(false)} />}
    </>
  )
}
