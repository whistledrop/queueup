import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-steam-connect-link-shortcut",
  date: "2026-11-10",
  title: "Make a desktop shortcut that joins your Rust server",
  description: "A Steam link can launch Rust straight into a server. How to build one for your server and put it on your desktop, so joining is a double-click.",
  body: `
If you play the same server every wipe, you can skip the menus entirely. Steam understands links that start a game and connect it to a server, and you can put one on your desktop.

## The link

For Rust, the link is:

**steam://run/252490//+connect 123.45.67.89:28015/**

Replace 123.45.67.89:28015 with your server's address, keeping the port after the colon. 252490 is Rust's ID on Steam, and everything after the double slash is passed to the game, telling it to connect. Your server's Discord or website will usually give the address.

## Make the desktop shortcut

1. Right-click an empty part of your desktop and choose **New**, then **Shortcut**.
2. For the location, paste your link (with your server's address in it).
3. Click **Next**, name it after the server, and click **Finish**.

Double-click it and Steam starts Rust and connects you, or puts you in the queue if the server is full.

> If Windows will not accept the link because of the space before the address, write the space as %20 instead: steam://run/252490//+connect%20123.45.67.89:28015/

## Things to know

- **Steam must be running and signed in.** If it is not, Windows usually starts it, and it signs in by itself if you ticked "remember me".
- **Close Rust first** if it is already open. The link is reliable when it starts the game, much less so when the game is already running.
- **If the server moves**, update the address in the shortcut's properties.

## Joining from somewhere else

A shortcut still needs you at your PC. QueueUp uses exactly this kind of Steam link, started from your phone: tap join, and your PC launches Rust into the server while you are somewhere else. See [how to join a Rust queue from your phone](/guides/join-rust-server-queue-from-your-phone).
`,
}

export default guide
