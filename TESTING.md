# Testing QueueUp for real

This is the guide for you, Logan, written to be followed step by step. No
programming knowledge is assumed. It has three parts:

- **Part A, the dress rehearsal.** Proves the whole system, phone to PC, with a
  pretend Rust. Do this first, from anywhere. If Part A works, everything except
  the game itself is known-good.
- **Part B, first contact with the real game.** The one-time checks that need
  you at the PC. About 30 minutes.
- **Part C, the real wipe-day test.**

Whenever something says "in a terminal on the PC": press the Windows key, type
`cmd`, press Enter, and type into the black window. Press Enter after each
command.

---

## Part A: the dress rehearsal (no Rust needed)

What you need: the relay and website deployed (docs/deploy.md), the
`QueueUpAgent.exe` file on the PC, and your phone.

1. **On your phone**, open the website and create an account.

2. **In a terminal on the PC** (use your real relay address):

   ```
   QueueUpAgent.exe pair --relay https://queueup-relay.fly.dev
   ```

   It shows a six character code.

3. **On your phone**, type the code into the box on the dashboard. The PC
   appears, named, with a red dot.

4. **In the terminal on the PC**, start the agent in pretend mode:

   ```
   QueueUpAgent.exe run --sim --scenario long_queue
   ```

   The pretend-Rust scenarios are built into the exe; nothing else to copy.

5. **On your phone**: the dot goes green, with an amber "Simulator" badge. Tap
   a server, tap Join. Watch: Launching, Connecting, In the queue with the
   position counting down, You're in.

6. **Turn on notifications** from the dashboard card and tap "Send a test". On
   an iPhone you must first add the site to your home screen (Share button,
   then "Add to Home Screen") and open it from there; that is an Apple rule,
   not ours.

7. **The reboot test.** Start another join. While it is mid-queue, restart the
   PC (a real restart). If Windows auto-login and the agent's autostart are set
   up (see prerequisites below), your phone should show: "Your PC went
   offline", then "Your PC is back online", and the join carries on by itself.
   You touch nothing.

8. **The cancel test.** Start a join, tap "Cancel and close Rust". It stops.

If all eight work, every part of QueueUp except the actual game is proven.

### Prerequisites on the PC (one-time, by hand)

These are Windows and Steam settings QueueUp deliberately does not touch:

- **Windows signs in automatically after a restart.** Search Windows settings
  for "netplwiz", untick "Users must enter a user name and password". Without
  this, a forced update leaves the PC on the lock screen, running nothing.
- **Steam starts with Windows and remembers your sign-in.** Steam Settings,
  Interface, "Run Steam when my computer starts", and stay signed in.
- **Sleep is off.** Settings, System, Power, set "Put my device to sleep" to
  Never. QueueUp does not wake sleeping PCs, by design.
- **The agent starts at login.** Right-click the QueueUp tray icon and tick
  "Start with Windows". (There is also a command for it,
  `QueueUpAgent.exe install-autostart`, if you prefer typing.)

QueueUp never asks for, sees, or stores your Steam password. Steam stays signed
in on the PC exactly as you left it.

---

## Part B: first contact with the real game (at the PC, once)

Two unknowns need real answers before wipe day. Both are described in detail in
`docs/steam-uri-test.md`; the short version:

1. **The launch link.** Try the five `steam://` links from that document, with
   Rust closed and with Rust open, and write down what happens. This tells us
   whether the way QueueUp starts the game actually works on a real machine.

2. **The log file.** Play one short session: join any server (ideally one with
   a queue), wait, get in, disconnect, close Rust. Then send me the file:

   ```
   %USERPROFILE%\AppData\LocalLow\Facepunch Studios LTD\Rust\Player.log
   ```

   (Paste that into the File Explorer address bar and copy the file out.)

   Until I have it, the agent's reading of the game's progress is built on
   guesses and it says so at startup. This file turns the guesses into facts,
   and I update one config file. **Do not skip this.**

3. Then a **real join of a quiet server** from your phone:

   ```
   QueueUpAgent.exe run
   ```

   (no `--sim`: real mode). Pick an emptyish server, tap Join. Rust should
   launch and connect on its own. Watch the phone track it.

If step 3 works, wipe day is the same thing with a queue in the middle.

---

## Part C: the real wipe-day test

The night before:

1. Check the dashboard: PC green, notifications on (send yourself a test).
2. Find your target server in QueueUp and save it.
3. Schedule the join: pick the server, set the time to a few minutes BEFORE the
   announced wipe time, and leave "Wait for the server to come back up" ticked.
   The time you pick is your local time wherever you are; the PC acts on the
   same instant regardless of where it is.
4. Leave the PC on, signed in, Steam running, Rust closed.

### Updating QueueUp on the PC

Do this on the gaming PC itself, in the browser there. You never need to copy a
file from your Mac.

**Your pairing is not in the program.** It lives in a separate settings file, so
replacing the program keeps the PC linked to your account. You will not have to
pair it again.

1. **Find where the current one lives.** Press Ctrl+Shift+Esc for Task Manager,
   find QueueUpAgent, right-click it and choose "Open file location". A folder
   opens. Leave that folder open.
2. **Quit the old one.** Look by the clock, bottom right. If you do not see the
   QueueUp icon, click the small "^" arrow: Windows hides new icons there.
   Right-click the icon and choose Quit. Windows will not let you replace the
   file while it is running.
3. **Download the new one.** Open https://queueuprust.netlify.app/download
   Windows or your browser will warn you about running a downloaded program.
   That is normal for any program that is not signed, which costs money we have
   not spent yet. Choose "Keep", then on the blue "Windows protected your PC"
   box choose "More info" and then "Run anyway".
4. **Put it in the same folder as the old one**, the one from step 1, and choose
   "Replace the file in the destination".

   This matters. Windows starts QueueUp from a remembered path, so if the new
   copy sits somewhere else, the old one keeps starting instead.
5. **Double-click it.** The icon comes back and, within a few seconds, the dot
   on the website turns green.

If you could not put it in the same folder, right-click the tray icon, untick
"Start with Windows", then tick it again. That points the remembered path at
wherever the new copy actually is.

### Force wipe also means a game update

On force wipe, Rust ships a client update with the wipe, so Steam has to
download several gigabytes before the game will start. QueueUp expects this:
it waits rather than giving up, and your phone shows the progress ("Steam is
updating Rust, 40% of 4.0 GB downloaded"). You do not need to do anything.

Two things make it faster, both worth doing the night before:

- In Steam, right-click Rust, Properties, Updates, set it to "Always keep this
  game updated". Then Steam grabs the update the moment it is released rather
  than when QueueUp asks for it.
- Leave Steam running. A sleeping Steam downloads nothing.
- Check the PC has room for the download, and that Steam is not in offline mode.
  QueueUp will tell you if the download stops moving, but the night before is a
  much better time to find out than ten minutes into the wipe.

On the day, watch your phone:

- At the scheduled time: "Scheduled join started".
- The server goes down for wipe; the timeline says so, and says it is watching.
- Within seconds of it coming back: connecting, then usually a queue.
- Milestones as you pass 100, 50, 10. Then: "You're in".
- Get to your PC when you can. Rust is sitting in the server, holding your slot.

If anything looks wrong: screenshot the phone, and afterwards grab the agent's
log via the tray icon ("Open the log file"). Those two things are enough for me
to work out what happened.

### What "wrong" looks like, and what it means

| The phone says | What it means |
|---|---|
| "Steam isn't logged in on your PC" | Open Steam on the PC once and sign in. QueueUp cannot do this for you. |
| "Your PC is offline" and stays that way | The PC is off, asleep, or lost internet. Autostart or auto-login may not be set. |
| "Rust closed unexpectedly", then retrying | The game crashed; QueueUp relaunches it and rejoins by itself. Only worry if it gives up. |
| "Tried several times and couldn't get in" | It ran out of retries. Check the timeline for the underlying reason and just tap Join again. |
| Stuck on "Waiting for the server" long after the server is visibly up | The server may have changed address in a way we could not follow. Cancel, search for the server again, Join. |
| "Steam is updating Rust" | Normal on force wipe. It waits and connects as soon as the download finishes. Nothing to do. |
| "Steam has paused the Rust download" | Open Steam on the PC and resume it. This will not fix itself. |
| "Steam has stopped downloading Rust" | Nothing has moved for several minutes. Check Steam on the PC: usually offline mode, a full disk, or a paused download. |
| "Steam isn't logged in on your PC" | Sign in to Steam on the PC. QueueUp tries twice before saying this, so a Steam that was merely restarting will already have sorted itself out. |
