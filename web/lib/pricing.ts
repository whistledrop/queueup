// One plan, one price. When Stripe arrives, its Price object mirrors this and
// the checkout call goes next to it; the landing page reads only from here.
export const PLAN = {
  currency: 'GBP',
  symbol: '£',
  monthly: 4.99,
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
export const BETA = true

/** What the landing page says about cost. */
export function costLine(): string {
  return BETA
    ? `Free while QueueUp is in beta. Try it and tell us how it went.`
    : `${priceLine()}. Setting up is free, you pay when you first join.`
}

export function priceLine(): string {
  return `${PLAN.symbol}${PLAN.monthly.toFixed(2)} a month`
}
