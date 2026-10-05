'use client'

// How long the discounted first month has left.
//
// Days and hours while there is more than a day to go, because "2 days 5
// hours 14 minutes" is a number nobody needs the minutes of. In the last day
// it switches to hours and minutes, because that is when the minutes start
// to matter.
//
// The deadline it counts to comes from the relay, and the relay is what
// enforces it at checkout. This only tells the truth about it: winding the
// clock on the phone back changes nothing about the price.

import { useEffect, useState } from 'react'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

function unit(n: number, one: string): string {
  return `${n} ${one}${n === 1 ? '' : 's'}`
}

/** "2 days 5 hours", "23 hours 14 minutes", "less than a minute". */
export function timeLeft(ms: number): string {
  if (ms < MINUTE) return 'less than a minute'
  if (ms >= DAY) {
    const days = Math.floor(ms / DAY)
    const hours = Math.floor((ms % DAY) / HOUR)
    return hours > 0 ? `${unit(days, 'day')} ${unit(hours, 'hour')}` : unit(days, 'day')
  }
  const hours = Math.floor(ms / HOUR)
  const minutes = Math.floor((ms % HOUR) / MINUTE)
  if (hours === 0) return unit(minutes, 'minute')
  return minutes > 0 ? `${unit(hours, 'hour')} ${unit(minutes, 'minute')}` : unit(hours, 'hour')
}

export default function Countdown({
  endsAt,
  onEnded,
  className,
}: {
  endsAt: Date
  /** Called once, the moment it reaches zero, so the price can be re-checked. */
  onEnded: () => void
  className?: string
}) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    // Fifteen seconds is often enough that the minute never sits visibly
    // stale in the last day, and rare enough to cost nothing.
    const t = setInterval(() => setNow(Date.now()), 15_000)
    return () => clearInterval(t)
  }, [])

  const left = endsAt.getTime() - now
  const ended = left <= 0

  useEffect(() => {
    if (ended) onEnded()
    // onEnded is deliberately left out: it is a fresh function every render
    // of the parent, and including it would call it over and over.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ended])

  if (ended) return null
  return (
    <p className={className} aria-live="polite">
      Offer ends in <b>{timeLeft(left)}</b>
    </p>
  )
}
