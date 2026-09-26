'use client'

// The help assistant as a bubble in the corner.
//
// Same endpoint and same assistant as the /ask page. The difference is reach:
// the people who most need it are the ones who have just hit a wall, and they
// do not go hunting through a footer for a help page. They are already looking
// at the screen that went wrong, so the help sits on it.

import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import s from './helpBot.module.css'

// Openers, so nobody faces an empty box wondering what it can do. These are
// the four things people actually got stuck on.
const examples = [
  'My PC says offline but it is switched on',
  'Windows will not let me open the download',
  'How do I set up for wipe day?',
  'Why is there no queue number?',
]

type Turn = { question: string; answer: string }

export default function HelpBot() {
  const [open, setOpen] = useState(false)
  const [question, setQuestion] = useState('')
  const [turns, setTurns] = useState<Turn[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const box = useRef<HTMLDivElement>(null)
  const field = useRef<HTMLTextAreaElement>(null)

  // Always show the newest answer, and start typing straight away on open.
  useEffect(() => {
    if (!open) return
    box.current?.scrollTo({ top: box.current.scrollHeight, behavior: 'smooth' })
  }, [open, turns, busy, error])

  useEffect(() => {
    if (open) field.current?.focus()
  }, [open])

  // Escape closes it, the way it closes everything else.
  useEffect(() => {
    if (!open) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open])

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

  if (!open) {
    return (
      <button className={s.launcher} onClick={() => setOpen(true)} aria-label="Ask for help">
        <Bubble />
        Help
      </button>
    )
  }

  return (
    <div className={s.panel} role="dialog" aria-label="Ask for help">
      <div className={s.head}>
        <div>
          <strong>Ask about QueueUp</strong>
          <p>It can see your PC and your last joins.</p>
        </div>
        <button className={s.close} onClick={() => setOpen(false)} aria-label="Close">
          ✕
        </button>
      </div>

      <div className={s.body} ref={box}>
        {turns.length === 0 && (
          <>
            <p className={s.opener}>
              Say what is happening and it will tell you what to do.
            </p>
            <div className={s.examples}>
              {examples.map((e) => (
                <button key={e} className={s.example} onClick={() => ask(e)} disabled={busy}>
                  {e}
                </button>
              ))}
            </div>
          </>
        )}

        {turns.map((t, i) => (
          <div key={i} style={{ display: 'contents' }}>
            <div className={s.question}>{t.question}</div>
            <div className={s.answer}>{t.answer}</div>
          </div>
        ))}

        {busy && <div className={s.answer}>Thinking...</div>}
        {error && <p className={s.problem}>{error}</p>}
      </div>

      <div className={s.foot}>
        <form
          className={s.form}
          onSubmit={(e) => {
            e.preventDefault()
            ask(question)
          }}
        >
          <textarea
            ref={field}
            value={question}
            onChange={(e) => setQuestion(e.target.value)}
            maxLength={2000}
            placeholder="What is happening?"
            // Enter sends, Shift+Enter is a new line: chat rules, because this
            // looks like a chat.
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault()
                ask(question)
              }
            }}
          />
          <button type="submit" className={s.send} disabled={busy || !question.trim()} aria-label="Send">
            ↑
          </button>
        </form>
        <p className={s.smallprint}>
          It only knows QueueUp and can get things wrong. Send feedback to reach
          a person.
        </p>
      </div>
    </div>
  )
}

function Bubble() {
  return (
    <svg width="17" height="17" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <path
        d="M21 12a8 8 0 0 1-8 8H5l-2 2v-9a8 8 0 0 1 8-8h2a8 8 0 0 1 8 7z"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinejoin="round"
      />
    </svg>
  )
}
