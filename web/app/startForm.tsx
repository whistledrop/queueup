'use client'

// Signing up, on the landing page, in one place.
//
// Email first, on its own. Press go and a password field drops in underneath
// it, and the account is made without ever leaving the page. The second page
// is gone: every page load between somebody wanting the thing and having it is
// a place where they stop wanting it.
//
// The address is kept the moment they press go, before the password exists.
// Somebody who gets that far and then thinks better of it used to be a visit
// that happened to nobody; now they are a person we can count and talk to.

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { storedPromo } from '@/lib/promo'

export default function StartForm({ className }: { className?: string }) {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [taken, setTaken] = useState(false)
  const passwordBox = useRef<HTMLInputElement>(null)

  // The field they are meant to fill in next should be the one their keyboard
  // is already pointed at.
  useEffect(() => {
    if (open) passwordBox.current?.focus()
  }, [open])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    const address = email.trim()
    if (!address || busy) return

    if (!open) {
      setBusy(true)
      try {
        // Keep the address before asking for anything else. If this fails we
        // carry on anyway: losing a lead is a shame, but refusing somebody a
        // sign-up because we could not file their address first is worse.
        await fetch('/api/relay/api/leads', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ email: address, code: storedPromo() }),
        })
      } catch {
        // as above
      }
      setBusy(false)
      setOpen(true)
      return
    }

    if (!password) return
    setBusy(true)
    setError('')
    setTaken(false)
    try {
      const res = await fetch('/api/auth/register', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email: address, password, code: storedPromo() }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        const message = body.error ?? 'That did not work. Try again.'
        setError(message)
        setTaken(/already/i.test(message))
        setBusy(false)
        return
      }
      router.push('/subscribe?welcome=1')
    } catch {
      setError('We could not reach QueueUp. Check your connection.')
      setBusy(false)
    }
  }

  return (
    <form className={className} onSubmit={submit}>
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
        {!open && (
          <button type="submit" disabled={busy} aria-label="Continue">
            <Arrow />
          </button>
        )}
      </div>

      {open && (
        <div className="startDrop">
          <input
            ref={passwordBox}
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Pick a password"
            autoComplete="new-password"
            aria-label="Pick a password"
          />
          <button type="submit" disabled={busy || !password} aria-label="Create account">
            <Arrow />
          </button>
        </div>
      )}

      {error && (
        <p className="startError">
          {error}{' '}
          {taken && (
            <Link href={`/login?email=${encodeURIComponent(email.trim())}`}>Sign in instead</Link>
          )}
        </p>
      )}

      {open && !error && (
        <p className="startSmall">
          Eight characters or more. By creating an account you have read the{' '}
          <Link href="/privacy">privacy notice</Link>.
        </p>
      )}
    </form>
  )
}

// The whole button, at the size a button that says one thing deserves.
function Arrow() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M5 12h13m-5.5-6L19 12l-6.5 6"
        stroke="currentColor"
        strokeWidth="2.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
