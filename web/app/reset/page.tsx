'use client'

// Choosing a new password, from the link in the email.

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'

export default function ResetPage() {
  return (
    <Suspense>
      <ResetForm />
    </Suspense>
  )
}

function ResetForm() {
  const token = useSearchParams().get('token') ?? ''
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState('')

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await fetch('/api/relay/api/auth/reset', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ token, password }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        setError(body.error ?? 'That did not work. Try again.')
        return
      }
      setDone(true)
    } catch {
      setError('We could not reach QueueUp. Check your connection.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
      </header>

      {error && <div className="error">{error}</div>}

      {!token ? (
        <div className="card">
          <h2>Something is missing from that link</h2>
          <p style={{ marginTop: 0 }}>
            Open the link from your email again, or ask for a new one.
          </p>
          <Link href="/forgot" className="btn btn-wide" style={{ textAlign: 'center' }}>
            Send me a new link
          </Link>
        </div>
      ) : done ? (
        <div className="card">
          <h2>Password changed</h2>
          <p style={{ marginTop: 0 }}>
            You can sign in with it now. Anywhere you were already signed in has
            been signed out, so use the new password there too.
          </p>
          <Link href="/login" className="btn btn-primary btn-wide" style={{ textAlign: 'center' }}>
            Sign in
          </Link>
        </div>
      ) : (
        <form className="card" onSubmit={submit}>
          <h2>Choose a new password</h2>
          <div className="stack">
            <div>
              <label htmlFor="password">New password</label>
              <input
                id="password"
                type="password"
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={8}
              />
            </div>
            <button type="submit" className="primary btn-wide" disabled={busy || password.length < 8}>
              {busy ? 'Saving' : 'Save new password'}
            </button>
          </div>
          <p className="muted small" style={{ marginBottom: 0 }}>
            At least 8 characters. Links work once, and for an hour.
          </p>
        </form>
      )}
    </div>
  )
}
