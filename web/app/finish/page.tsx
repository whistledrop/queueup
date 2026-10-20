'use client'

// The step after paying: choosing a password.
//
// Stripe sends everybody here. Somebody who signed up with just an email
// chooses their password now, which is the first moment the account holds
// something worth protecting — a subscription. Until they do, this browser is
// the only place they could ever get back in, so it comes before anything else.
//
// Anybody who already has a password — they made the account the old way, or
// came back to resubscribe — is passed straight through to the thank-you.

import { Suspense, useEffect, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { api, ApiError } from '@/lib/api'

function Finish() {
  const params = useSearchParams()
  const justPaid = params.get('subscribed') === '1'
  // Where to go once there is a password. Free players come here on their way
  // to the PC setup; paid players come here straight from Stripe. Only our own
  // paths are honoured, so a crafted link cannot bounce anybody off-site.
  const next = params.get('next')
  const onward = justPaid ? '/?subscribed=1' : next === 'get' ? '/get' : '/'

  const [email, setEmail] = useState('')
  const [ready, setReady] = useState(false)
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    ;(async () => {
      try {
        const me = await api<{ email: string; has_password?: boolean }>('/api/auth/me')
        if (me.has_password) {
          window.location.replace(onward)
          return
        }
        // No paywall bounce. This screen exists to give an account a
        // password, which a free player needs just as much as a paying one.
        setEmail(me.email)
        setReady(true)
      } catch (e) {
        // Signed out after paying, in a different browser. Their way back is
        // the reset link, which works for an account with no password yet.
        if (e instanceof ApiError && e.status === 401) window.location.replace('/login')
        else setError((e as Error).message)
      }
    })()
  }, [justPaid, onward])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!password || busy) return
    setBusy(true)
    setError('')
    try {
      await api('/api/auth/first-password', {
        method: 'POST',
        body: JSON.stringify({ password }),
      })
      window.location.replace(onward)
    } catch (e) {
      // Already set, from another tab: nothing left to do here.
      if (e instanceof ApiError && e.status === 409) {
        window.location.replace(onward)
        return
      }
      setError((e as Error).message)
      setBusy(false)
    }
  }

  return (
    <div className="shell narrow">
      <header className="top">
        <Link href="/" className="brand">
          Queue<span>Up</span>
        </Link>
      </header>

      <div className="card">
        {!ready && !error && <p className="muted">One moment</p>}
        {!ready && error && <div className="error">{error}</div>}

        {ready && (
          <form onSubmit={submit}>
            <h2>{justPaid ? "You're in. One last thing." : 'Choose a password'}</h2>
            <p style={{ marginTop: 0 }}>
              Choose a password, so you can sign in on your PC or a new phone.
            </p>

            {/* The address, as the username a password manager will save
                this password against. Without it they guess, and often
                guess wrong. */}
            <input type="email" value={email} autoComplete="username" readOnly hidden />

            <label htmlFor="pw">Password</label>
            <input
              id="pw"
              type="password"
              required
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              placeholder="Eight characters or more"
            />

            {error && <div className="error">{error}</div>}

            <button className="btn btn-primary btn-wide" type="submit" disabled={busy || !password}>
              {busy ? 'One moment' : 'Finish'}
            </button>
          </form>
        )}
      </div>
    </div>
  )
}

export default function FinishPage() {
  // useSearchParams needs a boundary, or the page is pushed into client
  // rendering at build time.
  return (
    <Suspense>
      <Finish />
    </Suspense>
  )
}
