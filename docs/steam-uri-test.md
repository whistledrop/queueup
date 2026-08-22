# Manual test: does the Steam launch link actually work?

**Do this on your gaming PC before you lose access to it. It takes about 15 minutes.**

Everything QueueUp does rests on one assumption: that Windows can be handed a
`steam://` link that launches Rust *and* connects it straight to a chosen server.
I have written the code so that this link is built in exactly one place
(`internal/game/steamuri.go`), so if the format is wrong it is a one-line fix.
But I need to know which format is the right one, and only you can find out.

## What you need first

- The IP and port of a Rust server that is **online and not full**. Any server
  will do. You can get it from the in-game server browser, or from
  battlemetrics.com (the address is shown as something like `51.83.128.10:28015`).
- Steam running and signed in.

Write that address down. Below, wherever you see `IP:PORT`, type your real one.

## The five links to try

Try them **in this order**. Stop as soon as one works, but note down which ones
you tried.

| # | Link to paste |
|---|---|
| A | `steam://run/252490//+connect IP:PORT/` |
| B | `steam://run/252490//+connect IP:PORT` |
| C | `steam://run/252490//+connect%20IP:PORT/` |
| D | `steam://rungameid/252490//+connect IP:PORT/` |
| E | `steam://connect/IP:PORT` |

## How to run one

1. Press `Windows key + R` (this opens the small "Run" box).
2. Paste the link in.
3. Press Enter.
4. Watch what happens.

## Test 1: with Rust CLOSED

Make sure Rust is not running. Close it if it is.

For each link, note down which of these happened:

- Nothing happened at all
- Steam popped up a box asking permission, and I clicked yes
- Rust launched, but landed on the main menu and did **not** connect
- Rust launched **and** connected (or went into the server's queue) — **this is the win**

## Test 2: with Rust ALREADY OPEN

This one matters just as much. Launch Rust normally and leave it sitting on the
main menu. Then run the same link that worked in Test 1, and note:

- Nothing happened
- The already-open Rust connected to the server
- A second copy of Rust tried to start
- Rust closed and reopened

Then repeat once more with Rust **already connected to a different server**, and
note whether it switched servers or ignored the link.

## What to send me

Copy this and fill it in:

```
Server I tested against: ____________________

TEST 1 (Rust closed)
  Link A: ____________________________________
  Link B: ____________________________________
  Link C: ____________________________________
  Link D: ____________________________________
  Link E: ____________________________________
  The link that worked: _______

TEST 2 (Rust already open on the main menu)
  Using link _______ : ________________________

TEST 3 (Rust already connected to a different server)
  Using link _______ : ________________________

Roughly how long from pressing Enter to being connected/queued: _______ seconds
Did Steam show a permission popup that needs clicking every time? yes / no
```

That last question is important. If Steam asks for confirmation on every launch,
the agent cannot answer it for you (clicking things automatically is exactly the
kind of thing we are not going to do), so we would need to find the Steam setting
that turns the prompt off and put it in the setup instructions.

## While you are at that PC, also note down

These save a second trip:

1. **Does this file exist?**
   `C:\Users\<your name>\AppData\LocalLow\Facepunch\Rust\Player.log`
   Paste the exact full path. Fastest way: press `Windows + R`, paste
   `%USERPROFILE%\AppData\LocalLow\Facepunch\Rust` and press Enter. A folder
   should open. Tell me what files are in it.

2. **Grab me a real Player.log.** Launch Rust, join a server that has a queue if
   you can find one, wait in the queue, join, then leave. Close Rust. Then copy
   that `Player.log` and send it to me. This is the single most valuable thing
   you can give me: the log parser is currently built on guesses, and this file
   turns all of them into facts.

3. **What is the Rust process called?** Open Task Manager (`Ctrl+Shift+Esc`),
   go to the Details tab while Rust is running, and find the Rust entry. Tell me
   the exact name (I have assumed `RustClient.exe`).
