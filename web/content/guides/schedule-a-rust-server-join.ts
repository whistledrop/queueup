import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "schedule-a-rust-server-join",
  date: "2026-11-07",
  title: "How to schedule a Rust server join for wipe night",
  description: "Schedule your PC to join a Rust server the moment it wipes, so you are in the first minutes even when you are not at home. How scheduled joins work and how to set one up.",
  body: `
Knowing when your server wipes is half the battle. The other half is being at your PC at that exact minute, which on a weekday evening is often impossible. A scheduled join takes that part away.

## What a scheduled join does

You choose a server and a time. At that time your gaming PC:

1. Starts watching the server.
2. Waits for it to **go down** for the wipe, so it does not join the old map minutes before it is deleted.
3. Waits for it to **come back** on the new map.
4. Launches Rust and **queues straight away**, while most people are still finding out it is up.

If the server had already wiped before your scheduled time, it sees that the server has just started and joins it straight away instead of waiting all night for a restart that already happened.

## Setting one up in QueueUp

1. Link your gaming PC (about two minutes, once).
2. Open **Schedule a wipe join** and pick your server.
3. Choose the evening, and a time a little before the wipe is due.
4. Leave your PC **on, awake and signed in to Steam**.

Your phone shows each step: waiting for the wipe, server down, server back, queued, in.

## Tips for wipe night

- **Schedule a little early.** The PC waits patiently; being early costs nothing.
- **Turn off sleep** first. A sleeping PC cannot join anything. See [keeping your PC awake](/guides/keep-your-pc-awake-for-rust-wipe).
- **On force wipe, let Steam update.** If Rust needs the monthly update, QueueUp waits for the download and shows its progress.

## Try one free

Free QueueUp accounts get **one free scheduled wipe join**, so you can try it on a real wipe night. Unlimited includes a scheduled join for every wipe.
`,
}

export default guide
