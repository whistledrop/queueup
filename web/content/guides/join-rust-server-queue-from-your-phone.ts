import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "join-rust-server-queue-from-your-phone",
  date: "2026-10-26",
  title: "How to join a Rust server queue from your phone",
  description: "You cannot play Rust on a phone, but you can start the queue from one. How remote queueing works, what your PC needs, and what it can and cannot do.",
  body: `
Rust is a PC game, so you cannot play it on your phone. But the slowest part of wipe night is not playing, it is **waiting in the queue**, and the queue does not need you sitting there. It only needs Rust running on your PC and connected to the server.

That is what remote queueing does: your phone tells your PC to join, and your PC does the waiting.

## How it works

With QueueUp:

1. A small app on your gaming PC stays signed in to your QueueUp account.
2. You pick a server on your phone and tap **Join**.
3. Your PC launches Rust through Steam, exactly as if you had clicked the server yourself, and joins the queue.
4. Your phone shows the progress: launching, connecting, queued, in the server.
5. If Rust crashes or drops out of the queue, the PC rejoins on its own.

By the time you get home, you are in.

## What your PC needs

- **Windows**, with **Steam** and **Rust** installed.
- **Switched on and awake.** A sleeping PC cannot start anything, so set Windows sleep to Never on nights you plan to use it.
- **Steam signed in.** QueueUp never asks for your Steam password; Steam has to be logged in already.

## Scheduling a join for wipe night

For wipes you know are coming, you can schedule a join instead of tapping at the right moment. Your PC watches the server go down for the wipe and come back, then queues the moment it does.

## Is it safe?

QueueUp opens Rust through a standard Steam link and reads the game's own log file to see what is happening. It does not read the game's memory, inject anything, press keys or change any game files. In the game, everything is still you.

## Free to try

QueueUp is free for 10 joins a month, plus one free scheduled wipe join, so you can try it on a real wipe before deciding whether you want more.
`,
}

export default guide
