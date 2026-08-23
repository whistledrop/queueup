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
    'Live queue position on your phone',
    'Notifications: milestones, you are in, PC offline',
    'One PC linked to your account',
  ],
} as const

export function priceLine(): string {
  return `${PLAN.symbol}${PLAN.monthly.toFixed(2)} a month`
}
