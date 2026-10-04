'use client'

// The first step of signing up, on the landing page itself.
//
// One field, not two. A button that says "Get QueueUp" asks somebody to
// commit before they have done anything; an email box asks for one small
// thing and carries them into the rest with momentum already behind them.
//
// And it is why we know anything about the people who leave. The address is
// kept the moment they press go, before the password, so somebody who starts
// and then thinks better of it is still a person we can count and talk to
// rather than a visit that happened to nobody.

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { storedPromo } from '@/lib/promo'

export default function StartForm({ className }: { className?: string }) {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)

  async function start(e: React.FormEvent) {
    e.preventDefault()
    const address = email.trim()
    if (!address || busy) return
    setBusy(true)
    try {
      // Keep it before going anywhere. If this fails we carry on regardless:
      // losing a lead is a shame, and blocking somebody from signing up
      // because we could not file their address first would be worse.
      await fetch('/api/relay/api/leads', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email: address, code: storedPromo() }),
      })
    } catch {
      // as above
    }
    router.push(`/login?mode=create&email=${encodeURIComponent(address)}`)
  }

  return (
    <form className={className} onSubmit={start}>
      <input
        type="email"
        required
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="you@email.com"
        autoComplete="email"
        aria-label="Your email"
      />
      <button type="submit" disabled={busy}>
        {busy ? 'One moment' : 'Get QueueUp'}
      </button>
    </form>
  )
}
