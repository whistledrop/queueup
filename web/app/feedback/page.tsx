'use client'

// Where beta testers tell us how it went.
//
// QueueUp is free during the beta precisely so that people will say this. So it
// has to take ten seconds: one box, one button, from any page, and it arrives
// with the account attached so nobody has to say who they are.

import { Suspense, useState } from 'react'
import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import Nav, { Footer } from '../nav'
import { api } from '@/lib/api'

export default function FeedbackPage() {
  return (
    <Suspense>
      <FeedbackForm />
    </Suspense>
  )
}

function FeedbackForm() {
  // Arriving from a finished join, the join is named so the note makes sense
  // on its own later.
  const params = useSearchParams()
  const jobId = params.get('job') ?? ''
  const server = params.get('server') ?? ''
  const [message, setMessage] = useState('')
  const [busy, setBusy] = useState(false)
  const [sent, setSent] = useState(false)
  const [error, setError] = useState('')

  async function send(e: React.FormEvent) {
    e.preventDefault()
    if (!message.trim()) return
    setBusy(true)
    setError('')
    const about = jobId ? `[about the join to ${server || 'a server'}, ${jobId}]\n` : ''
    try {
      await api('/api/feedback', {
        method: 'POST',
        body: JSON.stringify({ message: about + message.trim() }),
      })
      setSent(true)
      setMessage('')
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="shell">
      <Nav />

      {error && <div className="error">{error}</div>}

      {sent ? (
        <div className="card">
          <h2>Thank you</h2>
          <p style={{ marginTop: 0 }}>
            Got it. Every note gets read, and it is how QueueUp gets better
            before the beta ends.
          </p>
          <p className="muted">
            If something went wrong on the PC, a problem report helps even more:
            right-click the QueueUp icon by the clock and choose{' '}
            <b>Send a problem report to QueueUp</b>.
          </p>
          <div className="stack">
            <button className="btn-wide" onClick={() => setSent(false)}>
              Send another
            </button>
            <Link href="/" className="btn-wide" style={{ textAlign: 'center' }}>
              Back to QueueUp
            </Link>
          </div>
        </div>
      ) : (
        <form className="card" onSubmit={send}>
          <h2>How did it go?</h2>
          <p className="muted" style={{ marginTop: 0 }}>
            QueueUp is a free beta, and this is the price: tell us what worked,
            what did not, and what confused you. A sentence is plenty.
          </p>
          {jobId && (
            <p className="muted small">
              About your join to <b>{server || 'a server'}</b>. That gets sent
              along with your note.
            </p>
          )}
          <div className="stack">
            <label htmlFor="message" className="sr-only">Your feedback</label>
            <textarea
              id="message"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              maxLength={3800}
              placeholder="e.g. Scheduled a join for the 8pm wipe, got in at 8:02 while I was out. The queue screen confused me at first..."
            />
            <button type="submit" className="primary btn-wide" disabled={busy || !message.trim()}>
              {busy ? 'Sending' : 'Send feedback'}
            </button>
          </div>
          <p className="muted small" style={{ marginBottom: 0 }}>
            Something broken on the PC? Right-click the QueueUp icon by the clock
            and choose <b>Send a problem report to QueueUp</b> as well. It tells
            us exactly what happened.
          </p>
        </form>
      )}

      <Footer />
    </div>
  )
}
