'use client'

// The offer's deadline, as a clock you can watch run down, beside the price.
//
// Hours, minutes and seconds, the hours counted all the way up: a fresh offer
// reads "71:59:42", ticking every second. A number that visibly moves says
// "this ends" in a way a sentence underneath the price never does.
//
// The deadline comes from the relay — 72 hours from this person's own signup
// — and the relay enforces it at checkout. Winding the phone's clock back
// changes what this shows, and nothing about the price.

import { useEffect, useState } from 'react'

const SECOND = 1000
const MINUTE = 60 * SECOND
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

const two = (n: number) => String(n).padStart(2, '0')

/** Hours, minutes and seconds left: "71:59:42". */
export function clock(ms: number): string {
  const left = Math.max(0, ms)
  const hours = Math.floor(left / HOUR)
  const minutes = Math.floor((left % HOUR) / MINUTE)
  const seconds = Math.floor((left % MINUTE) / SECOND)
  return `${two(hours)}:${two(minutes)}:${two(seconds)}`
}

function unit(n: number, one: string): string {
  return `${n} ${one}${n === 1 ? '' : 's'}`
}

/** The same deadline in words, for screen readers: "2 days 5 hours". */
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
  labelClassName,
  digitsClassName,
}: {
  endsAt: Date
  /** Called once, the moment it reaches zero, so the price can be re-checked. */
  onEnded: () => void
  className?: string
  labelClassName?: string
  digitsClassName?: string
}) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    // A quarter of a second, not a whole one: an interval drifts, and a
    // one-second tick that drifts visibly skips a second now and then.
    const t = setInterval(() => setNow(Date.now()), 250)
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
    // role="timer" is announced only when asked, not every second; the
    // words in the label say it properly when it is.
    <span className={className} role="timer" aria-label={`Offer ends in ${timeLeft(left)}`}>
      <span className={labelClassName} aria-hidden="true">
        Ends in
      </span>
      <span className={digitsClassName} aria-hidden="true">
        {clock(left)}
      </span>
    </span>
  )
}
