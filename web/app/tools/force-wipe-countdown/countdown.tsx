'use client'

// The live part: a ticking countdown to the next force wipe, and each date in
// the visitor's own time zone. The server renders the page in UK time, so
// search engines and the first paint have real dates; this then takes over.

import { useEffect, useState } from 'react'
import s from '../tools.module.css'

function parts(ms: number) {
  const t = Math.max(0, Math.floor(ms / 1000))
  return { d: Math.floor(t / 86400), h: Math.floor((t % 86400) / 3600), m: Math.floor((t % 3600) / 60), s: t % 60 }
}

export function Countdown({ wipeISO }: { wipeISO: string }) {
  const wipe = new Date(wipeISO).getTime()
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now())
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])

  if (now === null) return <div className={s.units} style={{ minHeight: 64 }} />
  const left = wipe - now
  if (left <= 0) {
    return (
      <p className={s.big} style={{ margin: '6px 0 14px' }}>
        It&apos;s force wipe now.
      </p>
    )
  }
  const p = parts(left)
  return (
    <div className={s.units} aria-live="off">
      {(
        [
          [p.d, 'days'],
          [p.h, 'hours'],
          [p.m, 'minutes'],
          [p.s, 'seconds'],
        ] as const
      ).map(([n, label]) => (
        <div key={label} className={s.unit}>
          <b className={s.big}>{String(n).padStart(label === 'days' ? 1 : 2, '0')}</b>
          <span>{label}</span>
        </div>
      ))}
    </div>
  )
}

/** A date in the visitor's own zone, shown once the browser can say what that is. */
export function LocalTime({ iso, withDay = true }: { iso: string; withDay?: boolean }) {
  const [text, setText] = useState<string | null>(null)
  useEffect(() => {
    const d = new Date(iso)
    const fmt = new Intl.DateTimeFormat(undefined, {
      ...(withDay ? { weekday: 'long', day: 'numeric', month: 'long' } : {}),
      hour: 'numeric',
      minute: '2-digit',
      timeZoneName: 'short',
    })
    setText(fmt.format(d))
  }, [iso, withDay])
  return <>{text ?? '…'}</>
}
