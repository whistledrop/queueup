import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "what-to-do-while-waiting-in-a-rust-queue",
  date: "2026-11-19",
  title: "What to do while you wait in a Rust queue",
  description: "A long Rust queue does not need you staring at it. How to keep your place safe, and how to use the time, or not be there at all.",
  body: `
Wipe-night queues can be long. The queue does not need you watching it, but it does need a few things not to go wrong. Here is how to wait well.

## Keep your place safe

You lose your place if your connection ends. So while you wait:

- **Do not close Rust** or click another server.
- **Do not let the PC sleep.** See [keeping your PC awake](/guides/keep-your-pc-awake-for-rust-wipe).
- **Avoid heavy downloads** that could make your connection unstable.
- **Leave Windows Update alone** until you are in.

## Use the time

- **Plan your first hour**: where to build, what to farm. See the [wipe day checklist](/guides/rust-wipe-day-checklist).
- **Check the server's Discord** for map links, rules and announcements.
- **Get food, drinks, a charger**: once you are in, you will not want to leave.

## Have a backup

If the queue is not moving, it is worth knowing which other server you would try. But do not jump early: leaving puts you at the back of any queue you come back to.

## Or do not wait at all

The queue only needs Rust running on your PC. With QueueUp you can start the join from your phone, go and do something else, and see on your phone when you are in. If Rust crashes in the queue, it starts again and rejoins on its own.
`,
}

export default guide
