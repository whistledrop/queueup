// What people say, and room for what they will say.
//
// One real quote, from the person who built it, labelled as exactly that. It
// is here because it is the true story of why QueueUp exists — somebody in a
// classroom watching a wipe start without them — and because an honest
// founder's note reads better than a stranger's invented praise.
//
// Everything else waits. Real quotes go in QUOTES as they arrive, with a name
// or Discord handle and the server they play, and nothing goes in that was not
// actually said by somebody who actually paid.

import s from './landing.module.css'

type Quote = {
  text: string
  who: string
  /** Server or handle, shown under the name. Optional. */
  where?: string
}

const FOUNDER: Quote = {
  text:
    "I'm usually at school for when servers wipe. With this app I was able to join queue whilst in class, and by the time I was home my PC was in the server.",
  who: '9k-hour Rust player, founder of QueueUp',
}

/**
 * Real customer quotes. Add them here as they come in — nothing else needs
 * changing, and the section grows into a row on its own.
 *
 * Only with their permission, only their own words, and only somebody who
 * actually pays. An invented review is the one thing that would make every
 * true thing on this page worth less.
 */
const QUOTES: Quote[] = []

export default function Testimonials() {
  return (
    <section className={s.section}>
      <p className={s.kicker}>Why it exists</p>
      <h2>Wipe day waits for nobody</h2>
      <div className={QUOTES.length > 0 ? s.quoteGrid : undefined}>
        <blockquote className={s.quote}>
          <p>“{FOUNDER.text}”</p>
          <cite>
            — {FOUNDER.who}
            {FOUNDER.where && <span>{FOUNDER.where}</span>}
          </cite>
        </blockquote>
        {QUOTES.map((q) => (
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
