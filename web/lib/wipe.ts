// When Rust next restarts the world.
//
// Facepunch wipes on the first Thursday of each month, at 19:00 UK time — 7pm
// whether the country is on GMT or BST. That is the single busiest hour
// QueueUp has, and the reason most people hear about it at all.
//
// Worked out in UK time rather than the visitor's, because the wipe happens at
// 7pm in London no matter where they are reading. Somebody in New York needs
// to know it is 2pm their time, and the only way to get that right is to find
// the real instant and let their own browser show it.

/**
 * What o'clock it is in the UK at a given instant, as numbers.
 *
 * Asking the browser rather than doing the sums: it carries the timezone
 * rules, including which Sundays the clocks move, and those change by law.
 */
function ukParts(at: Date): { y: number; m: number; d: number; hh: number; weekday: number } {
  const f = new Intl.DateTimeFormat('en-GB', {
    timeZone: 'Europe/London',
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: 'numeric',
    hour12: false,
    weekday: 'short',
  })
  const part: Record<string, string> = {}
  for (const p of f.formatToParts(at)) part[p.type] = p.value
  const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
  return {
    y: Number(part.year),
    m: Number(part.month),
    d: Number(part.day),
    hh: Number(part.hour) % 24,
    weekday: days.indexOf(part.weekday),
  }
}

/** The instant at which a given UK wall-clock time happens. */
function ukInstant(y: number, m: number, d: number, hour: number): Date {
  // Start from the guess that UK time is UTC, then correct by however far out
  // that guess turns out to be. One pass is enough: the error is a whole
  // number of hours, and correcting by it lands exactly.
  const guess = new Date(Date.UTC(y, m - 1, d, hour))
  const seen = ukParts(guess)
  const drift = seen.hh - hour
  return new Date(guess.getTime() - drift * 3600_000)
}

/** The first Thursday of a given month, at 19:00 UK time. */
function wipeOf(year: number, month: number): Date {
  const first = new Date(Date.UTC(year, month - 1, 1))
  // 4 is Thursday. ((4 - weekday) + 7) % 7 days forward from the 1st.
  const day = 1 + ((4 - first.getUTCDay() + 7) % 7)
  return ukInstant(year, month, day, 19)
}

/**
 * The next force wipe after `now`, as a real instant.
 *
 * On wipe day itself this stays on today's 7pm until it has passed, so the
 * page counts down through the afternoon rather than jumping to next month.
 */
export function nextWipe(now: Date = new Date()): Date {
  const { y, m } = ukParts(now)
  const thisMonth = wipeOf(y, m)
  if (thisMonth.getTime() > now.getTime()) return thisMonth
  return m === 12 ? wipeOf(y + 1, 1) : wipeOf(y, m + 1)
}

/**
 * "Thursday 5 November, 20:00" — or "7:00 PM" for a reader whose locale uses
 * a twelve-hour clock. Their format, not ours.
 *
 * The minutes stay. Trimming ":00" to read "7pm" works in English and turns
 * "20:00" into a bare "20" everywhere that counts to twenty-four, which is
 * most of the places QueueUp is read.
 */
export function wipeWhen(at: Date): string {
  return (
    at.toLocaleDateString(undefined, { weekday: 'long', day: 'numeric', month: 'long' }) +
    ', ' +
    at.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  )
}
