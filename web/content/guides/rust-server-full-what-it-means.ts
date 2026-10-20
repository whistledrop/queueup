import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-server-full-what-it-means",
  date: "2026-11-08",
  title: "Rust server full? What it means and what to do",
  description: "A full Rust server does not reject you, it queues you. What a full server means on wipe night, how to tell a queue from a crash, and your options.",
  body: `
"Server is full" sounds like a rejection, but on most Rust servers it is not. When a server is at its player limit, you normally go into its **queue**, and get in when somebody leaves.

## Full means queue

On a full server, the loading screen shows that you are queued instead of loading in. That is normal. The wait depends on how quickly players leave, which right after a wipe is slowly. See [how Rust server queues work](/guides/how-rust-server-queues-work) for the details.

## When full really means no

A few servers set a limit on the queue itself, or turn queueing off. Then a full server really does turn new players away, and you may see a message saying the server is full when you try to connect. The only options are to try again later or pick another server.

## What to do on wipe night

- **Stay in the queue.** Leaving, or clicking another server, sends you to the back.
- **Do not restart Rust** unless it has actually frozen.
- **Check the server is not still starting.** Straight after a wipe, a server can appear and disappear a few times as it restarts. If it is not answering, wait a minute and connect again.
- **Have a backup server** in mind if the queue is enormous.

## Waiting without watching

The queue only needs Rust running on your PC, not you in front of it. With QueueUp you can start the join from your phone, leave your PC queuing, and check on your phone where it is up to. If it gets bounced out, it rejoins on its own.
`,
}

export default guide
