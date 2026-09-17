'use client'

// "I forgot my password."
//
// The answer is deliberately the same whether or not the address has an
// account: a page that says "no such account" is a way of finding out who uses
// QueueUp.

import { useState } from 'react'
import Link from 'next/link'

export default function ForgotPage() {
  const [email, setEmail] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState('')
  const [error, setError] = useState('')

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await fetch('/api/relay/api/auth/forgot', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        setError(body.error ?? 'That did not work. Try again.')
        return
      }
      setDone(body.status ?? 'Check your email.')
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
        <Link href="/login" className="btn quiet">Sign in</Link>
      </header>

      {error && <div className="error">{error}</div>}

      {done ? (
        <div className="card">
          <h2>Check your email</h2>
          <p style={{ marginTop: 0 }}>{done}</p>
          <p className="muted">
            Nothing after a few minutes? Look in spam, and check you typed the
            address you signed up with.
          </p>
          <Link href="/login" className="btn btn-wide" style={{ textAlign: 'center' }}>
            Back to sign in
          </Link>
        </div>
      ) : (
        <form className="card" onSubmit={submit}>
          <h2>Forgotten password</h2>
          <p className="muted" style={{ marginTop: 0 }}>
            Type the email address you signed up with and we will send you a
            link to choose a new password. It works for one hour.
          </p>
          <div className="stack">
            <div>
              <label htmlFor="email">Email</label>
              <input
                id="email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
            </div>
            <button type="submit" className="primary btn-wide" disabled={busy || !email}>
              {busy ? 'Sending' : 'Send me a link'}
            </button>
          </div>
        </form>
      )}
    </div>
  )
}
