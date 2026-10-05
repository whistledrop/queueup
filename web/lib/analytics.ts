'use client'

// What we measure, and the little we keep to measure it.
//
// QueueUp sells through videos, so the only question analytics has to answer
// is which video turned into which paying customer. That is the whole brief:
// where somebody came from, and how far down the line they got.
//
// It runs without cookies. `persistence: 'memory'` keeps PostHog's state in
// the tab and writes nothing to the device, which is what lets the site go
// without a consent banner. The cost is real and worth saying out loud: there
// is no durable visitor id, so somebody who comes back tomorrow is a new
// visitor, and a funnel cannot be followed across a reload. Counts of EVENTS
// are sound; counts of PEOPLE are not, and Stripe stays the only honest
// answer to how many customers there are.
//
// Nothing here identifies anybody: no email, no account id, no session
// recording. The attribution that the money is reconciled against is already
// recorded properly — the promo code goes onto the Stripe subscription and
// into the relay's own funnel. This is the looser, faster view on top.

import posthog from 'posthog-js'
import { storedPromo } from '@/lib/promo'

/** The events worth naming. Anything not on this list is not worth a chart. */
export type AnalyticsEvent =
  | 'account_created'
  | 'checkout_started'
  | 'subscription_paid'
  | 'agent_downloaded'

const KEY = process.env.NEXT_PUBLIC_POSTHOG_KEY ?? ''
const HOST = process.env.NEXT_PUBLIC_POSTHOG_HOST ?? 'https://eu.i.posthog.com'

let started = false

// The query parameters worth keeping. Everything else on the address is
// dropped, because a parameter nobody listed is a parameter nobody checked
// for somebody's email address in.
const CAMPAIGN_PARAMS = [
  'utm_source',
  'utm_medium',
  'utm_campaign',
  'utm_content',
  'utm_term',
  'utm_id',
  // Ad-platform click ids: how a click is matched to a spend.
  'gclid',
  'fbclid',
  'ttclid',
  'twclid',
  'msclkid',
] as const

/**
 * Reads the campaign and promo code off one address.
 *
 * The address is passed in rather than read from `window`, because the moment
 * we most need it — the start of a client-side navigation — is a moment when
 * `window.location` is still the page being left.
 */
function arrivedWith(href: string): Record<string, string> {
  const out: Record<string, string> = {}
  let url: URL
  try {
    url = new URL(href, window.location.origin)
  } catch {
    return out
  }
  for (const name of CAMPAIGN_PARAMS) {
    const value = url.searchParams.get(name)
    if (value && value.length <= 200) out[name] = value
  }
  const promo = (url.searchParams.get('promo') ?? url.searchParams.get('code') ?? '')
    .trim()
    .toUpperCase()
  if (promo && promo.length <= 64) out.promo_code = promo
  return out
}

/**
 * The promo code from an earlier page, or TikTok's if they are plainly in the
 * TikTok app — the same answer the paywall will get, so a chart here and a
 * sale there are talking about the same thing. This runs before the page has
 * had a chance to keep the code, so the TikTok check cannot wait for it.
 */
function savedPromo(): string {
  return storedPromo().slice(0, 64)
}

/**
 * Puts the campaign and promo code on every event from now on.
 *
 * Only ever adds. An internal link carries no utm_source, and clearing them
 * on the first click would throw away the campaign for every event after it —
 * which is every event that actually matters. A later address that DOES carry
 * them overwrites, because last click wins: somebody who arrives on one link
 * and comes back on another belongs to the second, the same rule the promo
 * code itself follows.
 */
function remember(href: string): void {
  const found = arrivedWith(href)
  if (!found.promo_code) {
    // Not on this address, but possibly caught on an earlier one. Registering
    // it again is harmless and covers the common path: land on /?promo=X,
    // click through to the paywall, pay from there.
    const saved = savedPromo()
    if (saved) found.promo_code = saved
  }
  if (Object.keys(found).length > 0) posthog.register(found)
}

/** Starts PostHog. Safe to call twice; does nothing at all without a key. */
export function startAnalytics(): void {
  if (started || typeof window === 'undefined' || !KEY) return
  posthog.init(KEY, {
    api_host: HOST,
    // Events go through our own subdomain, so PostHog has to be told where
    // its own app lives, or the links it builds point at the proxy.
    ui_host: 'https://eu.posthog.com',

    // No cookies and no localStorage: PostHog's state lives in the tab and
    // dies with it. This is the line that keeps the site free of a consent
    // banner, and also why visitor counts here cannot be trusted.
    persistence: 'memory',

    // Without a durable visitor, a person profile would be created on every
    // page load: a thousand strangers who are all the same person. Profiles
    // are only made on identify(), which this site never calls.
    person_profiles: 'identified_only',

    // Replay records what somebody typed and looked at. That is a different
    // kind of data entirely, it genuinely does need consent, and no question
    // worth asking about this site needs it.
    disable_session_recording: true,

    // Pageviews are sent by hand, from the router. Client-side navigation
    // changes the address without a page load, so anything automatic either
    // misses those or double-counts them.
    capture_pageview: false,
    capture_pageleave: true,

    // PostHog starts these on its own, and none of them were asked for.
    // Heatmaps and dead clicks record where on a page somebody moved and
    // clicked, which is a good deal more than "which video did they come
    // from", and all three cost events on a plan billed by the event.
    disable_surveys: true,
    capture_heatmaps: false,
    capture_dead_clicks: false,

    // The privacy page says that "do not track" stops this, so it has to.
    // PostHog ignores the setting unless asked, and a privacy page that is
    // not true is worse than no privacy page.
    respect_dnt: true,
  })
  started = true
}

/** One pageview, for an address that may not be the current one yet. */
export function trackPageview(href: string): void {
  if (!started) return
  remember(href)
  posthog.capture('$pageview', { $current_url: new URL(href, window.location.origin).href })
}

/**
 * One named event.
 *
 * Properties are listed explicitly and never spread in from somewhere else: an
 * event is a thing somebody reads off a chart, and the way personal data gets
 * into one is by spreading a response into it without looking.
 */
export function track(
  event: AnalyticsEvent,
  properties?: Record<string, string | number | boolean>,
  options?: { leaving?: boolean },
): void {
  if (!started) return
  // `leaving` is for an event fired a moment before the page goes away — off
  // to Stripe, or off to the agent download. Events are batched by default,
  // and a batch still waiting its turn when the browser navigates is a batch
  // that never arrives. Sending instantly uses a keepalive request, which
  // survives the navigation. These are the two events in the funnel that
  // would otherwise be the ones most likely to go missing.
  posthog.capture(event, properties, options?.leaving ? { send_instantly: true } : undefined)
}
