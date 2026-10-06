'use client'

// Signing up, on the landing page: an email address and nothing else.
//
// The next screen is the price. The password comes after they have paid,
// which is the first moment there is anything in the account worth
// protecting; asking for it sooner is asking for effort before they know
// whether they want the thing.
//
// The same box is how somebody gets back. They signed up inside TikTok's own
// browser, tapped the reminder email, and landed here in Safari signed out:
// typing the address again takes them back to the price. Only ever into an
// account that has never been paid for and never had a password. Anything
// more than that needs the password, or a link sent to their own inbox.

import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { claimCode, storePromo } from '@/lib/promo'
import { PLAN } from '@/lib/pricing'
import { track } from '@/lib/analytics'

export default function StartForm({
  className,
  from = 'landing',
}: {
  className?: string
  /** Which form on the page this is, so the two can be told apart later. */
  from?: string
}) {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [signIn, setSignIn] = useState(false)
  const [sent, setSent] = useState('')
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    const address = email.trim()
    if (!address || busy) return
    setBusy(true)
    setError('')
    setSignIn(false)
    setSent('')
    // Kept as well as sent, so the paywall shows the discount they just
    // claimed rather than working it out again.
    const code = claimCode()
    storePromo(code)
    try {
      const res = await fetch('/api/auth/start', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email: address, code }),
      })
      const body = await res.json().catch(() => ({}))
      if (res.status === 201 || res.status === 200) {
        if (body.created) track('account_created', { from })
        router.push(body.created ? '/subscribe?welcome=1' : '/subscribe')
        return
      }
      if (res.status === 202) {
        // Paid but never chose a password: the way back is a link in their
        // own inbox, and this is all the page says about it.
        setSent(body.status ?? 'Check your email for a link.')
        setBusy(false)
        return
      }
      setError(body.error ?? 'That did not work. Try again.')
      setSignIn(Boolean(body.sign_in))
      setBusy(false)
    } catch {
      setError('We could not reach QueueUp. Check your connection.')
      setBusy(false)
    }
  }

  return (
    <form className={className} onSubmit={submit}>
      {/* Above the field, because an empty box asking for an email offers
          nothing in return, and this is the reason to fill it in. Shown to
          everybody, because everybody gets it: somebody with no code of
          their own claims with the site's. No price here, and no "first
          month" either; the paywall spells out £1.99 then £4.99 before
          anybody can be charged. */}
      <p className="startClaim">
        Claim {percentOff}% off
        <DownArrow />
      </p>
      <div className="startRow">
        <input
          type="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          placeholder="you@email.com"
          autoComplete="email"
          aria-label="Your email"
        />
        <button type="submit" disabled={busy} className="startClaimBtn">
          Claim
        </button>
      </div>

      {error && (
        <p className="startError">
          {error}{' '}
          {/* No address in the link: an email in a URL ends up in browser
              history and in the logs of every server it passes through. */}
          {signIn && <Link href="/login">Sign in</Link>}
        </p>
      )}

      {sent && <p className="startSmall">{sent}</p>}

      {/* Pressing Claim is what makes the account now, so this is where
          the privacy notice has to be: before it, not after. */}
      {!error && !sent && (
        <p className="startSmall">
          By continuing you accept the <Link href="/privacy">privacy notice</Link>.
        </p>
      )}
    </form>
  )
}

// Worked out rather than written down, so it cannot drift from the real
// prices if either one ever moves.
const percentOff = Math.round((1 - PLAN.intro / PLAN.monthly) * 100)

// Ties the offer to the box underneath it, so the two read as one thing
// rather than a claim floating above an unrelated field.
function DownArrow() {
  return (
    <svg width="13" height="13" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M12 5v13m-6-5.5L12 19l6-6.5"
        stroke="currentColor"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
