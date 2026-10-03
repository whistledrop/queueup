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

/** Reads ?promo= (or ?code=) off the current address and keeps it. */
export function capturePromoFromURL(): void {
  if (typeof window === 'undefined') return
  const params = new URLSearchParams(window.location.search)
  const code = (params.get('promo') ?? params.get('code') ?? '').trim().toUpperCase()
  if (!code || code.length > 64) return
  try {
    window.localStorage.setItem(KEY, code)
  } catch {
    // Private windows and blocked storage: the code is lost, which costs us
    // attribution on one sale and nothing else. Never worth an error.
  }
}

/** The code to pre-fill and send at checkout, if we have one. */
export function storedPromo(): string {
  if (typeof window === 'undefined') return ''
  try {
    return window.localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

export function storePromo(code: string): void {
  try {
    window.localStorage.setItem(KEY, code.trim().toUpperCase())
  } catch {
    // as above
  }
}
