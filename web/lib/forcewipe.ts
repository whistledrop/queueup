// Rust's force wipe: the first Thursday of every month, when Facepunch
// releases the monthly update, usually around 7pm UK time.
//
// The update goes out once, worldwide, so there is one moment for everybody;
// what differs between regions is only the local clock, and when individual
// community servers choose to open their new map afterwards. Everything here
// works in that one UK moment and leaves local times to the visitor's browser.

/** The usual hour of the update, UK time. A typical time, not a promise. */
export const UPDATE_HOUR_UK = 19

/** The first Thursday of a month (month is 0-11), as a calendar day. */
export function firstThursday(year: number, month: number): number {
  const weekday = new Date(Date.UTC(year, month, 1)).getUTCDay() // 0 = Sunday
  return 1 + ((4 - weekday + 7) % 7)
}

/** The instant it is `hour`:00 in the UK on that day, whatever the clocks are doing. */
function ukMoment(year: number, month: number, day: number, hour: number): Date {
  // The UK is UTC in winter and UTC+1 in summer: try both, keep the one the
  // UK itself agrees with.
  for (const offset of [0, 1]) {
    const d = new Date(Date.UTC(year, month, day, hour - offset))
    const ukHour = Number(
      new Intl.DateTimeFormat('en-GB', { timeZone: 'Europe/London', hour: 'numeric', hour12: false }).format(d),
    )
    if (ukHour === hour) return d
  }
  return new Date(Date.UTC(year, month, day, hour))
}

/** The usual force wipe moment in a given month. */
export function forceWipe(year: number, month: number): Date {
  return ukMoment(year, month, firstThursday(year, month), UPDATE_HOUR_UK)
}

/**
 * The next force wipes from `now`, soonest first. A wipe stays "next" for
 * three hours after its usual time, while servers are still coming back.
 */
export function upcomingForceWipes(now: Date, count: number): Date[] {
  const out: Date[] = []
  // Start from last month, so a wipe from earlier this evening still counts.
  for (let i = -1; out.length < count; i++) {
    const first = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth() + i, 1))
    const wipe = forceWipe(first.getUTCFullYear(), first.getUTCMonth())
    if (wipe.getTime() + 3 * 3600_000 > now.getTime()) out.push(wipe)
  }
  return out
}
