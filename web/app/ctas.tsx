'use client'

// The ways in, further down the page.
//
// The hero has the email box, but somebody who reads all the way to the end of
// Fair play has just had their last objection answered and is a long scroll
// away from anything to press. These put the next step where the decision
// actually gets made.
//
// Each one goes to the Price card's form rather than a different page: the
// address they type is what starts an account, and there is no reason to make
// them load anything to type it.

import { track } from '@/lib/analytics'
import s from './landing.module.css'

/** Takes them to the form in the Price card, and puts the cursor in it. */
function goToForm() {
  const section = document.getElementById('pricing')
  section?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  // After the scroll, not during: focusing first would yank the page there
  // instantly and undo the scroll that explains where they have arrived.
  window.setTimeout(() => {
    const forms = document.querySelectorAll<HTMLInputElement>('#pricing input[type="email"]')
    forms[0]?.focus({ preventScroll: true })
  }, 600)
}

/** A "Get QueueUp" button, which says where on the page it was pressed. */
export function CtaRow({ where }: { where: string }) {
  return (
    <div className={s.ctaRow}>
      <button
        type="button"
        className={s.cta}
        onClick={() => {
          track('cta_clicked', { position: where })
          goToForm()
        }}
      >
        Get QueueUp
      </button>
    </div>
  )
}

/**
 * The bar along the bottom of a phone.
 *
 * Only on a phone, and only once the hero's own form has been scrolled past —
 * two buttons asking for the same thing on one screen is one too many. It is
 * CSS that hides it on a desktop, so there is no guessing about screen sizes
 * in JavaScript.
 */
export function StickyCta() {
  return (
    <div className={s.sticky}>
      <button
        type="button"
        className={s.stickyBtn}
        onClick={() => {
          track('cta_clicked', { position: 'sticky' })
          goToForm()
        }}
      >
        Get QueueUp
      </button>
    </div>
  )
}
