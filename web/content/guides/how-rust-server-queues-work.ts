import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "how-rust-server-queues-work",
  date: "2026-10-24",
  title: "How Rust server queues work, and why you are stuck in one",
  description: "When a Rust server is full you join a queue instead of the server. Here is how that queue works, what moves you up it, and what makes you lose your place.",
  body: `
When a Rust server reaches its player limit, it does not turn you away. It puts you in a **queue**, and lets you in when somebody leaves. On a popular server on wipe day that queue can be hundreds of people long.

## What the queue actually is

Every Rust server has a maximum number of players. When it is full, new connections wait in line in the order they arrived. Each time a player leaves the server, the person at the front of the queue gets their slot.

So how fast the queue moves depends entirely on **how fast people leave**. On an ordinary evening that is steady. Straight after a wipe, almost nobody leaves, because everybody has just arrived and is busy building, which is why wipe-day queues can sit almost still for a while.

## Where you see your position

While you wait, Rust's loading screen shows that you are queued rather than loading in. Before you click, the server browser tells you whether a server is full, and a full server on wipe night almost always means a queue.

## What makes you lose your place

Your place belongs to your connection. If it ends, you go to the back of the line. That happens when:

- **You close Rust**, or it crashes.
- **Your PC goes to sleep** or restarts, including for a Windows update.
- **Your internet drops** for long enough for the server to give up on you.
- **You click on a different server.** There is one queue per connection.

This is the frustrating part: a queue that took forty minutes to climb can be lost in a second.

## Can you skip the queue?

On official servers, no. Everybody waits in the same line.

Some community servers sell or give **queue skip** (sometimes called priority queue) as a perk for supporters, which puts those players ahead of everybody else. If a server does this, it usually says so on its website or Discord.

## Waiting without sitting there

The queue itself does not need you. It only needs Rust open and connected on your PC. That is the idea behind QueueUp: you tap join on your phone, your PC launches Rust and waits in the queue, and if Rust crashes it rejoins on its own. By the time you sit down, you are in.

## The short version

- A full server queues new players and lets them in as others leave.
- Wipe-day queues move slowly because nobody leaves just after a wipe.
- Closing Rust, a crash, sleep or a dropped connection puts you back at the start.
- Some community servers sell queue skip; official servers do not.
`,
}

export default guide
