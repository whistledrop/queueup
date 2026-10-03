'use client'

// Catches ?promo= on whatever page somebody lands on.
//
// A TikTok link goes to queueuprust.com/?promo=TIKTOK, but a server's Discord
// might point at /help or straight at /login. The code has to be caught
// wherever they arrive and survive until they reach the paywall, which may be
// twenty minutes and six pages later.

import { useEffect } from 'react'
import { capturePromoFromURL } from '@/lib/promo'

export default function PromoCatcher() {
  useEffect(() => {
    capturePromoFromURL()
  }, [])
  return null
}
