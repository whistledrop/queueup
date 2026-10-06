// Why QueueUp exists, in the words of the person who built it.
//
// It sits before How it works because it is the reason for everything after
// it: somebody in a classroom watching a wipe start without them. It is the
// quote itself, at full size, with no heading over it — a title would only be
// us describing what the reader is about to read for themselves.
//
// Real customer quotes go in QUOTES as they arrive: their own words, their
// name or Discord handle, the server they play, and only from somebody who
// actually pays. Nothing invented, ever.

import s from './landing.module.css'

type Quote = {
  text: string
  who: string
  /** The server they play on, or their Discord handle. Optional. */
  where?: string
}

const FOUNDER: Quote = {
  text:
    "I'm usually at school for when servers wipe. With this app I was able to join queue whilst in class, and by the time I was home my PC was in the server.",
  who: '9k-hour Rust player, founder',
}

/** Real customer quotes. Add them here; the section grows to fit. */
const QUOTES: Quote[] = []

export default function Testimonials() {
  const all = [FOUNDER, ...QUOTES]
  return (
    <section className={s.section}>
      <div className={all.length > 1 ? s.quoteGrid : undefined}>
        {all.map((q) => (
          <blockquote className={s.quote} key={q.text}>
            <p>“{q.text}”</p>
            <cite>
              — {q.who}
              {q.where && <span>{q.where}</span>}
            </cite>
          </blockquote>
        ))}
      </div>
    </section>
  )
}
