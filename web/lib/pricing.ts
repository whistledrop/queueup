// One plan, one price. When Stripe arrives, its Price object mirrors this and
// the checkout call goes next to it; the landing page reads only from here.
export const PLAN = {
  currency: 'GBP',
  symbol: '£',
  monthly: 4.99,
  // The discounted first month is NOT a standing offer. It exists only behind
  // a promo code, because the code is the only thing that tells us which
  // channel a customer came from, and a discount everybody gets automatically
  // answers nothing. See lib/promo.ts.
  intro: 1.99,
  name: 'QueueUp',
  includes: [
    'Unlimited joins, any Rust server',
    'Scheduled wipe day joins',
    'Live status on your phone, from launch to you are in',
    'Rejoins on its own if Rust crashes or the PC reboots',
    'One PC linked to your account',
  ],
} as const

// BETA is true while QueueUp is handed out free in exchange for feedback. The
// relay's billing gate is off at the same time (QUEUEUP_BILLING unset), so
// nothing anywhere asks for money. Turn both off together when charging starts.
export const BETA = false

/** What the landing page says about cost. */
export function costLine(): string {
  return BETA
    ? `Free while QueueUp is in beta. Try it and tell us how it went.`
    : `${priceLine()}. Cancel anytime, one-click refund if it doesn't work on your setup.`
}

/** The first-month offer, spelled out in full: what it costs now AND after. */
export function introLine(): string {
  return `${PLAN.symbol}${PLAN.intro.toFixed(2)} for your first month, then ${priceLine()}`
}

export function priceLine(): string {
  return `${PLAN.symbol}${PLAN.monthly.toFixed(2)} a month`
}
