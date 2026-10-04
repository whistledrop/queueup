// Analytics, started before the app is interactive.
//
// Next runs this file after the document loads and before React hydrates,
// which is the earliest a pageview can be sent and the only place it can be
// sent once per navigation without guessing.
//
// The alternative — a client component reading useSearchParams in the root
// layout — drags the whole site into client rendering and needs a Suspense
// boundary around it to build at all. This does the same job with neither.
//
// Everything is wrapped: a failure to count a pageview must never be a
// failure to show the page.

import { startAnalytics, trackPageview } from '@/lib/analytics'

try {
  startAnalytics()
  // The first pageview. onRouterTransitionStart only fires on navigations
  // AFTER this one, so without this the landing page — the page every
  // campaign actually points at — would never be counted.
  trackPageview(window.location.href)
} catch {
  // No analytics this session. Nothing else about the page changes.
}

// Every client-side navigation after the first.
//
// This fires as the navigation starts, so window.location is still the page
// being left: the address has to come from the argument, not the browser.
export function onRouterTransitionStart(url: string) {
  try {
    trackPageview(url)
  } catch {
    // as above
  }
}
