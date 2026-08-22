'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'

export default function LoginPage() {
  const router = useRouter()
  const [creating, setCreating] = useState(false)
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
      router.push('/')
      router.refresh()
    } catch {
      setError('We could not reach QueueUp. Check your connection.')
      setBusy(false)
    }
  }

  return (
    <>
      <header className="top">
        <span className="brand">Queue<span>Up</span></span>
      </header>

      <div className="card">
        <h2>{creating ? 'Create an account' : 'Sign in'}</h2>
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
    </>
  )
}
