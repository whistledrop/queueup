import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "how-long-are-rust-queues-on-wipe-day",
  date: "2026-11-17",
  title: "How long are Rust queues on wipe day?",
  description: "Why some wipe-day queues take minutes and others take hours, what makes a queue move, and how to tell before you join whether you are in for a long wait.",
  body: `
There is no single answer, because a queue moves only as fast as people leave the server. But you can usually tell what you are in for.

## What makes a queue move

Each time a player leaves a full server, the next person in the queue gets in. So a queue moves quickly when people come and go, and slowly when nobody leaves.

Right after a wipe, almost nobody leaves. Everybody has just arrived and is busy. That is why wipe-night queues on popular servers can barely move for a while, then speed up later in the evening as people log off.

## Signs of a long wait

- **A big, popular server** that was full the moment it came back.
- **Force wipe night**, when everybody plays at once.
- **Joining late**, after the first rush has already filled it.
- **A server with queue skip**, where supporters go ahead of you. See [queue skip and priority queue](/guides/rust-queue-skip-and-priority-queue).

## Signs of a short one

- **Joining in the first minutes** after the server comes back.
- **A smaller or newer server.**
- **Later in the wipe**, when people come and go more.

## How to spend less time waiting

1. **Be early.** Being in before the server fills beats any queue.
2. **Do not leave and rejoin.** You go to the back every time.
3. **Let your PC wait for you.** The queue does not need you at the desk; QueueUp keeps your PC queuing while you are elsewhere and rejoins if it gets bounced.
`,
}

export default guide
