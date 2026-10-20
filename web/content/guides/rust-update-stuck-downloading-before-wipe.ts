import type { Guide } from '@/lib/guides'

const guide: Guide = {
  slug: "rust-update-stuck-downloading-before-wipe",
  date: "2026-11-03",
  title: "Rust update stuck before wipe? How to get it downloading",
  description: "On force wipe every Rust player needs the update at once. What to try when the Rust update will not start, stalls, or Steam keeps saying it is queued.",
  body: `
On force wipe, everybody needs the same update at the same moment, and Steam can struggle. Here is what to try, in order, when the Rust update will not start or stalls.

## Before anything else

**Start Steam early.** Steam often begins the download as soon as the update is published. Being signed in a little before the usual time means you are downloading while others are still opening Steam.

## If the update says "queued"

Steam downloads one game at a time. Open the **Downloads** page (at the bottom of the Library) and move Rust to the top, or pause the other downloads.

## If it is stuck at 0 or barely moving

1. **Pause and resume** the download. It sometimes kicks it into life.
2. **Change your download region.** Steam, Settings, Downloads, then Download Region. A server under heavy load in your region may be slower than one a country away.
3. **Clear the download cache** from the same Downloads settings page. Steam restarts and may ask you to sign in again, so have your password ready.
4. **Restart Steam fully**: exit it from the system tray rather than closing the window, then open it again.

## If it finishes but Rust will not start

- **Verify the game files.** In your Library, right-click Rust, Properties, Installed Files, then "Verify integrity of game files". It re-downloads anything damaged.
- **Restart your PC** if Easy Anti-Cheat complains; anti-cheat updates sometimes need one.

## If you will not be at your PC

Updates are one of the things that make wipe night hard to time. QueueUp shows the update's progress on your phone while Steam downloads it, and joins the server as soon as the game is ready.
`,
}

export default guide
