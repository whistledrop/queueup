import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-blueprint-wipe-vs-map-wipe",
  date: "2026-10-31",
  title: "Rust blueprint wipe vs map wipe: what is the difference?",
  description: "A map wipe resets the world; a blueprint wipe resets what you know how to craft. How the two differ, how often each happens, and what it means for your next wipe.",
  body: `
Rust has two kinds of wipe, and servers often do them on different schedules. Knowing which is which tells you how much of a fresh start you are really getting.

## Map wipe

A **map wipe** resets the world. Every base, every box and every item on the server is gone, and many servers start a new map with a different layout.

Map wipes are what server schedules usually describe: "weekly" means the map resets weekly.

## Blueprint wipe

A **blueprint (BP) wipe** resets what you have learned to craft. In Rust you unlock blueprints by researching items or working through the tech tree, and those blueprints stay with you on that server across map wipes until a BP wipe clears them.

After a BP wipe, everybody is back to crafting only the basics.

## How often each happens

- **Map wipes** follow the server's schedule: weekly, biweekly or monthly.
- **Blueprint wipes** are often less frequent, typically on **force wipe**, the first Thursday of the month. A server advertising "Weekly | BP monthly" resets the map each week but keeps blueprints until force wipe.
- **On force wipe**, the map always resets on every server. Official servers reset blueprints too; community servers choose, and most do.

## What it means for you

- **Map wipe only:** you lose your base and loot, but keep your crafting progress. Early game is quick, because you can craft good gear straight away.
- **Map and BP wipe:** a true fresh start. Everyone is equal, and the early hours matter even more.

That is why a full force wipe is the most even playing field Rust offers, and why the first minutes on the server count. If you want to be there for them, see [when Rust force wipe is](/guides/when-is-rust-force-wipe).
`,
}

export default guide
