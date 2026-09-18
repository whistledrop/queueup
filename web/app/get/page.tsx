'use client'

// queueuprust.com/get: the page people open on their PC, from the link they
// emailed themselves or the address their phone told them to type. It does one
// job: get QueueUp onto this PC and running, then send them back to their phone.

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { SmartScreenHelp } from '../dashboard'

export default function GetPage() {
  const [notWindows, setNotWindows] = useState(false)
  useEffect(() => {
    const ua = navigator.userAgent
    setNotWindows(!/Windows/i.test(ua) && /Mac|Linux|Android|iPhone|iPad/i.test(ua))
  }, [])

  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/help" className="btn quiet">Help</Link>
      </header>

      <div className="card">
        <h2>Set up QueueUp on this PC</h2>
        {notWindows && (
          <div className="notice">
            Open this page on the <b>Windows PC</b> you play Rust on. QueueUp
            runs there; your phone is the remote.
          </div>
        )}
        <ol className="setup">
          <li>
            <strong>Download QueueUp.</strong>
            <a className="btn btn-primary" href="/download" style={{ marginTop: 8 }}>
              Download for Windows
            </a>
          </li>
          <li>
            <strong>Put it somewhere permanent and double-click it.</strong>
            <span className="muted small">
              A folder like C:\QueueUp is ideal. Not Downloads: it lives there
              from now on.
            </span>
            <SmartScreenHelp />
          </li>
          <li>
            <strong>It shows a six character code. Type it into QueueUp on your phone.</strong>
            <span className="muted small">
              That links the two. From then on, tap Join on your phone and this PC
              does the rest.
            </span>
          </li>
        </ol>
      </div>

      <p className="muted small" style={{ textAlign: 'center' }}>
        No account yet? <Link href="/login?mode=create">Create one</Link>, then
        type the code there.
      </p>
    </div>
  )
}
