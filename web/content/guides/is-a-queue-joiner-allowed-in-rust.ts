import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "is-a-queue-joiner-allowed-in-rust",
  date: "2026-11-03",
  title: "Is it allowed to use a queue joiner in Rust?",
  description: "What a remote queue tool like QueueUp actually does on your PC, how that differs from a cheat or a bot, and what to check on the server you play.",
  body: `
It is the first question anybody should ask about any tool for a game with anti-cheat: is this going to get me banned? Here is a plain answer for QueueUp, and how to judge any other tool.

## What a cheat does

Cheats and bots work by changing or reading the game itself: reading game memory to see through walls, injecting code, sending inputs to aim or play for you, or editing game files. Easy Anti-Cheat, which Rust uses, exists to detect exactly those things.

## What QueueUp does

QueueUp does four things on your PC, all of them things you could do with a mouse:

- **Opens Rust through Steam**, using the same kind of link a bookmark or a server's website would use.
- **Reads the log file** Rust writes anyway, to know whether you are connecting, queued or in.
- **Checks whether Rust is running**, so it can start it again after a crash.
- **Closes Rust** when you press cancel.

It never reads the game's memory, injects anything, presses keys or moves the mouse, and it does not touch any game files. Once you are in the server, everything you do in game is you.

## Server rules are the server's

Facepunch's anti-cheat is one thing; each **server's own rules** are another. Community servers can set their own rules, and a few have opinions about queue tools or "AFK" players. If you play somewhere with strict rules, check them, or ask an admin. Most servers care about what you do in game, not how you launched it.

## How to judge any tool

Ask what it does on your PC:

- Does it need to run **inside** the game, or alongside it?
- Does it read or change **game memory or files**?
- Does it **send input** to the game?

If the answer to any of those is yes, it is the kind of thing anti-cheat is built to catch. A tool that only starts the game and reads its log is doing what you already do by hand.
`,
}

export default guide
