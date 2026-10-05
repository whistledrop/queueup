'use client'

// The promo code somebody arrived with.
//
// Codes are the whole attribution system: every channel gets its own, and a
// sale with no code is a sale we cannot trace. Somebody arriving on
// queueuprust.com/?promo=TIKTOK must never have to type it, and must still
// have it twenty minutes later when they finally reach the paywall, so it is
// caught on whatever page they land on and kept until checkout.
//
// Last click wins. Somebody who sees a video, forgets, and comes back through
// a server's Discord link belongs to the Discord link: it is the one that
// actually moved them, and the alternative is paying two partners for one
// customer.

const KEY = 'queueup_promo'

/** The code given to somebody who is plainly here from TikTok but whose link carried no code. */
export const TIKTOK_CODE = 'TIKTOK'

/**
 * Whether this visit is happening inside the TikTok app.
 *
 * A link tapped in TikTok opens in TikTok's own browser, and that browser
 * names itself: "musical_ly" on iPhone, "trill" on some Androids, and
 * "BytedanceWebview" on both, after the company that makes it. Failing that,
 * a visit that arrived from tiktok.com says so in the referrer — though only
 * on the first page, which is why the answer is kept rather than asked again.
 *
 * It catches most of TikTok, not all of it: somebody who taps "open in
 * browser" lands in Safari looking like anybody else. That is why the bio
 * link carries the code itself, and this is the net underneath it.
 */
export function fromTikTok(): boolean {
  if (typeof navigator === 'undefined') return false
  if (/musical_ly|trill_|BytedanceWebview|TikTok/i.test(navigator.userAgent ?? '')) return true
  try {
    const from = document.referrer ? new URL(document.referrer).hostname : ''
    return /(^|\.)tiktok\.com$/i.test(from)
  } catch {
    return false
  }
}

/** Reads ?promo= (or ?code=) off the current address and keeps it. */
export function capturePromoFromURL(): void {
  if (typeof window === 'undefined') return
  const params = new URLSearchParams(window.location.search)
  const code = (params.get('promo') ?? params.get('code') ?? '').trim().toUpperCase()
  if (code && code.length <= 64) {
    store(code)
    return
  }
  // No code in the link. If they are plainly here from TikTok and have no
  // code already, they get TikTok's — so a bare queueuprust.com in a bio, a
  // comment or a DM still lands on the offer the videos promise, and still
  // counts as TikTok. A code they did arrive with always wins: a creator's or
  // a server's own code is not overwritten just because the link that carried
  // it was posted on TikTok.
  if (!read() && fromTikTok()) store(TIKTOK_CODE)
}

/** The code to pre-fill and send at checkout, if we have one. */
export function storedPromo(): string {
  if (typeof window === 'undefined') return ''
  // Where storage is blocked (private windows), nothing above could be kept,
  // so somebody inside TikTok is recognised again each time it is asked.
  return read() || (fromTikTok() ? TIKTOK_CODE : '')
}

export function storePromo(code: string): void {
  store(code.trim().toUpperCase())
}

function read(): string {
  try {
    return window.localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

function store(code: string): void {
  try {
    window.localStorage.setItem(KEY, code)
  } catch {
    // Private windows and blocked storage: the code is lost, which costs us
    // attribution on one sale and nothing else. Never worth an error.
  }
}
