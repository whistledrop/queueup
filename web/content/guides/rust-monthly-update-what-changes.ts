import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-monthly-update-what-changes",
  date: "2026-11-18",
  title: "The Rust monthly update: what it changes and why it wipes",
  description: "Every first Thursday Facepunch releases a Rust update, and every server wipes for it. What the monthly update contains, where to read about it, and how to be ready.",
  body: `
Rust gets a big update every month, released on the **first Thursday**. It is the reason force wipe exists: the update changes the game enough that every server has to start a fresh map to run it.

## What a monthly update usually contains

Each one is different, but they typically mix:

- **New content**: items, building parts, monuments or vehicles.
- **Balance changes** to weapons, raiding and progression.
- **Fixes and performance** improvements.
- **Map and world changes**, sometimes.

## Where to read about it

Facepunch publishes a **devblog** about each update on the official Rust website, usually around release time, and the patch notes appear in Steam's news for the game. Community YouTubers and Discords also summarise the changes on the day.

## Why every server wipes

Server files from the old version cannot always be loaded by the new one, and changes to the game can break old maps and saves. Rather than leave every server to manage that, the monthly update forces everyone to start fresh, which is why it is called a **force** wipe. See [when Rust force wipe is](/guides/when-is-rust-force-wipe).

## Being ready on the day

- **Start Steam early** so the update downloads as soon as it is out. See [update stuck before wipe](/guides/rust-update-stuck-downloading-before-wipe).
- **Know your server's opening time.** It is usually soon after the update lands, around 7pm UK time.
- **Join as soon as it is back.**

If the update lands while you are not at your PC, QueueUp can wait for Steam to finish downloading it and then join your server, showing the progress on your phone.
`,
}

export default guide
