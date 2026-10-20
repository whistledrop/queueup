'use client'

// Paste a server address, get the Steam link that joins it. The same link
// QueueUp uses to start Rust into a server.

import { useMemo, useState } from 'react'
import s from '../tools.module.css'

const ADDRESS = /^([a-zA-Z0-9.-]+):(\d{2,5})$/

export default function Maker() {
  const [raw, setRaw] = useState('')
  const [copied, setCopied] = useState('')

  const parsed = useMemo(() => {
    // Forgiving about what people paste: "client.connect 1.2.3.4:28015",
    // a steam:// link, or the address on its own.
    const cleaned = raw
      .trim()
      .replace(/^client\.connect\s+/i, '')
      .replace(/^steam:\/\/\S*connect(\s|%20)+/i, '')
      .replace(/\/$/, '')
    const m = ADDRESS.exec(cleaned)
    if (!m || Number(m[2]) > 65535) return null
    return `${m[1]}:${m[2]}`
  }, [raw])

  const link = parsed ? `steam://run/252490//+connect ${parsed}/` : ''
  const shortcutLink = parsed ? `steam://run/252490//+connect%20${parsed}/` : ''
  const consoleLine = parsed ? `client.connect ${parsed}` : ''

  async function copy(text: string, what: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(what)
      setTimeout(() => setCopied(''), 2000)
    } catch {
      setCopied('')
    }
  }

  return (
    <div className={s.panel}>
      <label htmlFor="addr">Server address</label>
      <input
        id="addr"
        value={raw}
        onChange={(e) => setRaw(e.target.value)}
        placeholder="123.45.67.89:28015"
        autoComplete="off"
        spellCheck={false}
      />
      {raw.trim() && !parsed && (
        <p className={s.error}>
          That doesn&apos;t look like a server address. It should be an IP or host, a colon, and the
          port, like 123.45.67.89:28015.
        </p>
      )}
      {parsed && (
        <>
          <h3 style={{ margin: '20px 0 8px', fontSize: 16 }}>One-click join link</h3>
          <code className={s.code}>{link}</code>
          <div className={s.row}>
            <a className={s.btn} href={shortcutLink} style={{ textDecoration: 'none' }}>Join now</a>
            <button className={s.ghost} onClick={() => copy(link, 'link')}>
              {copied === 'link' ? 'Copied' : 'Copy link'}
            </button>
          </div>
          <h3 style={{ margin: '22px 0 8px', fontSize: 16 }}>For a desktop shortcut</h3>
          <code className={s.code}>{shortcutLink}</code>
          <div className={s.row}>
            <button className={s.ghost} onClick={() => copy(shortcutLink, 'shortcut')}>
              {copied === 'shortcut' ? 'Copied' : 'Copy for a shortcut'}
            </button>
          </div>
          <h3 style={{ margin: '22px 0 8px', fontSize: 16 }}>For the F1 console</h3>
          <code className={s.code}>{consoleLine}</code>
          <div className={s.row}>
            <button className={s.ghost} onClick={() => copy(consoleLine, 'console')}>
              {copied === 'console' ? 'Copied' : 'Copy command'}
            </button>
          </div>
        </>
      )}
    </div>
  )
}
