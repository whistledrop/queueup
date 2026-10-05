'use client'

// Where the unsubscribe link in every reminder email lands.
//
// It asks before it acts. Opening this page does nothing on its own: the
// button does. That is not friction for its own sake — workplace and school
// email systems open every link in a message to check it is safe, and a page
// that unsubscribed on opening would quietly take people off the list who
// never asked. One tap is the price of that not happening.
//
// No sign-in. The token in the link is what proves it is theirs, and
// somebody who wants fewer emails should not have to remember a password to
// get them.

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'

function Unsubscribe() {
  const params = useSearchParams()
  const account = params.get('a') ?? ''
  const token = params.get('t') ?? ''
  const [state, setState] = useState<'ready' | 'busy' | 'done' | 'failed'>('ready')
  const [error, setError] = useState('')

  async function stop() {
    setState('busy')
    setError('')
    try {
      const res = await fetch('/api/relay/api/unsubscribe', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ a: account, t: token }),
      })
      if (res.ok) {
        setState('done')
        return
      }
      const body = await res.json().catch(() => ({}))
      setError(body.error ?? 'That did not work. Try the link again in a minute.')
      setState('failed')
    } catch {
      setError('We could not reach QueueUp. Check your connection and try again.')
      setState('failed')
    }
  }

  const broken = !account || !token

  return (
    <div className="shell narrow">
      <header className="top">
        <Link href="/" className="brand">
          Queue<span>Up</span>
        </Link>
      </header>

      <div className="card">
        {state === 'done' ? (
          <>
            <h2>You&apos;re unsubscribed</h2>
            <p style={{ marginTop: 0 }}>
              No more reminder emails. This is permanent: nothing turns it back on.
            </p>
            <p className="muted small">
              You&apos;ll still get emails you ask for yourself, like a password reset.
            </p>
          </>
        ) : broken ? (
          <>
            <h2>This link isn&apos;t complete</h2>
            <p style={{ marginTop: 0 }}>
              Some of it got lost on the way here. Try the unsubscribe link from the email
              again, or <Link href="/feedback">tell us</Link> and we&apos;ll take you off by
              hand.
            </p>
          </>
        ) : (
          <>
            <h2>Stop the reminder emails?</h2>
            <p style={{ marginTop: 0 }}>
              We&apos;ll stop sending reminders about your QueueUp offer.
            </p>
            {error && <div className="error">{error}</div>}
            <button className="btn btn-primary btn-wide" onClick={stop} disabled={state === 'busy'}>
              {state === 'busy' ? 'One moment' : 'Unsubscribe'}
            </button>
          </>
        )}
      </div>
    </div>
  )
}

export default function UnsubscribePage() {
  // useSearchParams needs a boundary to build, or the whole page is pushed
  // into client rendering at build time.
  return (
    <Suspense>
      <Unsubscribe />
    </Suspense>
  )
}
