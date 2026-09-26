'use client'

// The help assistant.
//
// It knows what is true of this person's account, so the first answer is
// usually the real one rather than a list of things to try. The two escape
// hatches are always visible: the help page, and a person.

import { useState } from 'react'
import Link from 'next/link'
import Nav, { Footer } from '../nav'
import { api } from '@/lib/api'

// Openers, so nobody faces an empty box wondering what it can do.
const examples = [
  'My PC says offline but it is switched on',
  'Windows will not let me open the download',
  'How do I set up for wipe day?',
  'Why is there no queue number?',
]

type Turn = { question: string; answer: string }

export default function AskPage() {
  const [question, setQuestion] = useState('')
  const [turns, setTurns] = useState<Turn[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function ask(text: string) {
    const q = text.trim()
    if (!q || busy) return
    setBusy(true)
    setError('')
    setQuestion('')
    try {
      const res = await api<{ answer: string }>('/api/support/ask', {
        method: 'POST',
        body: JSON.stringify({ question: q }),
      })
      setTurns((list) => [...list, { question: q, answer: res.answer }])
    } catch (e) {
      setError((e as Error).message)
      setQuestion(q)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="shell">
      <Nav />

      {error && <div className="error">{error}</div>}

      <div className="card">
        <h2>Ask about QueueUp</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          It can see whether your PC is connected and what your last joins did,
          so say what is happening and it will tell you what to do.
        </p>

        {turns.length === 0 && (
          <div className="chips" style={{ marginBottom: 4 }}>
            {examples.map((e) => (
              <button key={e} className="chip" onClick={() => ask(e)} disabled={busy}>
                {e}
              </button>
            ))}
          </div>
        )}

        {turns.map((t, i) => (
          <div key={i} className="server">
            <div className="askQuestion">{t.question}</div>
            <div className="askAnswer">{t.answer}</div>
          </div>
        ))}

        {busy && <p className="muted" style={{ marginBottom: 0 }}>Thinking...</p>}

        <form
          className="stack"
          style={{ marginTop: 14 }}
          onSubmit={(e) => {
            e.preventDefault()
            ask(question)
          }}
        >
          <textarea
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            maxLength={2000}
            placeholder="What is happening?"
            style={{ minHeight: 80 }}
          />
          <button type="submit" className="primary btn-wide" disabled={busy || !question.trim()}>
            {busy ? 'Thinking' : 'Ask'}
          </button>
        </form>
      </div>

      <p className="muted small" style={{ textAlign: 'center' }}>
        It only knows QueueUp, and it can get things wrong. The{' '}
        <Link href="/help">help page</Link> has the same answers written out, and
        the <Link href="/feedback">feedback page</Link> reaches a person.
      </p>

      <Footer />
    </div>
  )
}
