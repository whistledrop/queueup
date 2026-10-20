import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-queue-skip-and-priority-queue",
  date: "2026-11-01",
  title: "Rust queue skip and priority queue: what they are",
  description: "Some Rust community servers let supporters skip the queue. How queue skip works, where you find it, and what to do on servers where nobody can skip.",
  body: `
On a full Rust server, everybody waits in the same line. Except, on some servers, the people who do not.

## What queue skip is

**Queue skip** (also called priority queue or VIP queue) lets certain players go to the front of a server's queue, or straight in. It is not a Rust feature as such: community servers add it with server plugins, and decide who gets it.

## Who gets it

- **Supporters.** Many community servers offer queue skip as a perk for donating or subscribing to help pay for the server.
- **Regulars and staff.** Some give it to admins, moderators or long-standing players.
- **Nobody.** Plenty of servers do not use it at all.

**Official Facepunch servers have no queue skip.** Everybody waits.

## Where to find out

A server with queue skip will usually advertise it on its website, store page or Discord, often alongside the other supporter perks. If it is not mentioned anywhere, assume there is none.

## Is it worth it?

That depends on how often you hit a queue. If you play one busy server every wipe and the queue regularly costs you the first hour, it might be. If you hop between servers, or queues are rare where you play, probably not.

## The alternative: let the queue wait for you

A queue is mostly a problem because you have to sit in front of it. If your PC waits in the queue while you are somewhere else, the length of the line matters far less. That is what QueueUp does: you tap join on your phone, your PC queues, and it works on any server, with or without queue skip.
`,
}

export default guide
