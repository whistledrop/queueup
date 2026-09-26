'use client'

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  )
}

function LoginForm() {
  const router = useRouter()
  const params = useSearchParams()
  // The landing page's buttons land people straight on the create form.
  const wantsCreate = params.get('mode') === 'create'
  // Somebody who has just deleted their account arrives here, and being
  // dropped at a sign-in page with no word about it reads like a bug.
  const justDeleted = params.get('deleted') === '1'
  const [creating, setCreating] = useState(wantsCreate)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await fetch(creating ? '/api/auth/register' : '/api/auth/login', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email, password }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        setError(body.error ?? 'That did not work. Try again.')
        setBusy(false)
        return
      }
      if (creating) {
        const billing = await fetch('/api/relay/api/billing')
          .then((r) => (r.ok ? r.json() : null))
          .catch(() => null)
        if (billing && billing.enabled && !billing.subscribed && billing.checkout_ready) {
          router.push('/subscribe?welcome=1')
          return
        }
      }
      router.push('/')
      router.refresh()
    } catch {
      setError('We could not reach QueueUp. Check your connection.')
      setBusy(false)
    }
  }

  return (
    <div className="shell">
      <header className="top">
        <span className="brand">Queue<span>Up</span></span>
      </header>

      <div className="card">
        <h2>{creating ? 'Create an account' : 'Sign in'}</h2>
        {justDeleted && (
          <div className="notice">
            <b>Your account is deleted.</b> Everything we held about it is gone.
            You are welcome back any time.
          </div>
        )}
        {error && <div className="error">{error}</div>}
        <form onSubmit={submit} className="stack">
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
          <div>
            <label htmlFor="password">Password</label>
            <input
              id="password"
              type="password"
              autoComplete={creating ? 'new-password' : 'current-password'}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          {creating && (
            <p className="muted small" style={{ margin: 0 }}>
              QueueUp is a free beta. Creating an account means you have read
              the <a href="/privacy">privacy notice</a>: we keep your email and
              what your joins did, and never ask for your Steam password.
            </p>
          )}
          <button type="submit" className="primary btn-wide" disabled={busy}>
            {busy ? 'One moment' : creating ? 'Create account' : 'Sign in'}
          </button>
        </form>
      </div>

      <button
        className="btn-wide"
        onClick={() => { setCreating(!creating); setError('') }}
      >
        {creating ? 'I already have an account' : 'Create an account'}
      </button>

      {!creating && (
        <p className="muted small" style={{ textAlign: 'center', marginTop: 14 }}>
          <Link href="/forgot">Forgotten your password?</Link>
        </p>
      )}
    </div>
  )
}
