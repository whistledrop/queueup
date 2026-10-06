'use client'

// How long until Rust restarts the world.
//
// The first Thursday of the month at 7pm UK, which is the busiest hour
// QueueUp has and the reason most people are reading the page at all. A real
// date, counted properly, in the reader's own timezone — never a number that
// resets when the page does.

import { useEffect, useState } from 'react'
import { nextWipe, wipeWhen } from '@/lib/wipe'
import s from './landing.module.css'

const two = (n: number) => String(n).padStart(2, '0')

export default function WipeClock() {
  // Nothing on the server: it would render that machine's idea of the time
  // and then disagree with the browser a moment later.
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now())
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])

  if (now === null) return null

  const at = nextWipe(new Date(now))
  const left = at.getTime() - now
  const days = Math.floor(left / 86_400_000)
  const hours = Math.floor((left % 86_400_000) / 3_600_000)
  const minutes = Math.floor((left % 3_600_000) / 60_000)
  const seconds = Math.floor((left % 60_000) / 1000)

  return (
    <div className={s.wipeClock}>
      <p className={s.wipeClockLabel}>Next force wipe</p>
      <p className={s.wipeClockDigits}>
        {days > 0 && (
          <>
            <b>{days}</b>
            <small>d</small>
          </>
        )}
        <b>{two(hours)}</b>
        <small>h</small>
        <b>{two(minutes)}</b>
        <small>m</small>
        <b>{two(seconds)}</b>
        <small>s</small>
      </p>
      {/* Shown in their own timezone, because the instant is the same
          everywhere even though the clock on the wall is not. */}
      <p className={s.wipeClockWhen}>{wipeWhen(at)}, your time</p>
    </div>
  )
}
