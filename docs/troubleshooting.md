# Everything that goes wrong, and what fixes it

The single source of truth for QueueUp support: the help page, the messages the
app shows, and any support bot all come from here. Every entry is a real
problem somebody has actually hit, not a guess.

Format: **what the person sees** → why → what to tell them. Keep it that way
round. Nobody arrives knowing which part broke; they arrive knowing what is on
their screen.

---

## Setting up

### "Windows protected your PC" when I run the download
Normal, and expected. QueueUp is not code signed yet, which costs money that
has not been spent. Windows shows this for anything it does not recognise.

**Fix:** click **More info** (small underlined text on the left, easy to miss),
then **Run anyway**. Once only.

### "Smart App Control blocked this app"
A stricter Windows 11 feature, usually on brand-new machines. It behaves two
ways: often it warns and still lets you continue, sometimes it refuses flat.

**Fix if it refuses:** Windows Security → App & browser control → Smart App
Control settings → **Off**. Since the April 2026 Windows update it can be
switched back on afterwards; on a PC missing that update, switching it off is
permanent without reinstalling Windows, so update Windows first if they care.

### My browser says the file "isn't commonly downloaded"
Same cause as above: an unsigned file few people have downloaded yet.
**Fix:** Keep / Keep anyway in the browser's download list.

### The pairing code does not work
Codes last **ten minutes**. **Fix:** close QueueUp on the PC, start it again,
use the fresh code. Codes are typed into the website on the phone, not into
the PC.

### I am on a Mac / console
QueueUp runs on the Windows PC that runs Rust. There is nothing to install on
a phone, and no Mac version, because Rust has no Mac version. The website
works on any phone or computer.

---

## "My PC is offline" when the PC is switched on

This is the most common support question by a distance. Work down the list.

1. **Is the QueueUp icon by the clock?** Click the small **^** arrow: Windows
   hides new icons there. No icon means QueueUp is not running. Double-click
   `QueueUpAgent.exe` to start it.
2. **Did the file move?** "Start with Windows" remembers the exact path the
   file was at when it was ticked. Move the file afterwards and Windows keeps
   trying to start something that is no longer there, so nothing runs after a
   restart. Starting it by hand repairs the entry automatically. **Best fix:**
   put it in `C:\QueueUp`, start it from there, then tick Start with Windows.
   Never leave it in Downloads or on the Desktop, both of which Windows tidies.
3. **Has Windows switched the startup entry off?** Settings → Apps → Startup,
   find QueueUp, set it **On**. Windows does this to apps it does not
   recognise, which is another thing code signing would cure.
4. **Is the PC actually awake?** A sleeping PC looks exactly like a switched-off
   one from the website. See sleep, below.
5. **Is the PC online?** Open any website on it.
6. **Does the tray icon say "Stopped"?** Quit it and start it again.

The dot on the dashboard goes green within a few seconds of QueueUp starting.

### The PC keeps going offline on its own
Sleep. QueueUp cannot wake a sleeping PC, by design, and a PC that dozes off
twenty minutes before wipe simply misses it.

**Fix:** Settings → System → Power → "When plugged in, put my device to sleep
after" → **Never**. Letting the screen turn off is fine. QueueUp reads this
setting itself and warns on the dashboard when it would cause trouble.

---

## Joining

### Stuck on "Connecting" and never reaches the queue
Connecting genuinely takes a couple of minutes: Rust and Easy Anti-Cheat are
slow, and the map has to load. If it sits much longer: look at the PC for an
error or a Steam window; if Rust is not running at all, Steam may be
mid-update, which the phone says. Cancel and try again; if it happens twice,
send a problem report.

### "Rust didn't start on your PC. Something there may be waiting for a click"
Something on the PC wants a human. The usual cause is a Windows permission box
from Steam, which appears when Steam installs the pieces that ship with the
game, Easy Anti-Cheat among them. Ordinary patches do not ask; a patch
carrying a new anti-cheat can.

**Fix:** go to the PC, click through whatever is waiting, start the join again.

### "Steam isn't running on your PC"
Start Steam and sign in. QueueUp waits 90 seconds before saying this, so a
Steam that was merely restarting will have sorted itself out.

### "Steam isn't logged in"
Sign in to Steam on the PC. QueueUp retries twice before saying this, because
Steam restarting looks identical for a few seconds.

### "Steam is updating Rust"
Normal, especially on force wipe. QueueUp waits as long as it takes and
connects when it finishes. **Nothing to do.** Two things make it quicker next
time, both worth doing the night before: set Rust to "Always keep this game
updated", and leave Steam running, since a closed Steam downloads nothing.

### "Steam has paused / stopped downloading Rust"
This one needs a person at the PC; waiting will not fix it. Open Steam →
Downloads. Usually: the download is paused (resume it), Steam is in offline
mode (Steam menu → Go Online), or the drive is full.

### The server refused the connection
That came from the server, not QueueUp: a password, a whitelist, a ban, or a
full server that is not taking a queue. QueueUp does not retry a refusal,
because a refusal does not fix itself.

### It says I left the server / closed Rust, but I did not
Closing Rust, disconnecting to the main menu, or backing out of the queue all
end the join deliberately: QueueUp takes that as "I have changed my mind" and
will not drag you back in. If the app said that while you did none of them,
that is a bug worth a problem report, with the time it happened.

### Rust crashed mid-queue
QueueUp relaunches it and rejoins on its own. You lose your place in the queue,
because the game did, but nothing needs doing.

### Windows restarted mid-queue
The join survives it. QueueUp starts with Windows, reconnects and picks the job
back up by itself, as long as Windows signs in automatically.

### The join just stopped after hours
Joins waiting for a PC that never came back are closed after **six hours**, so
an ancient job cannot fire up Rust days later. Start it again whenever the PC
is on.

---

## The queue

### Why is there no queue position?
Because nothing outside the game knows it. Rust does not write your place in
the queue to its log or publish it anywhere. QueueUp used to show a number
worked out from the server's total queue length, which was misleading: that is
how long the line is, not where you are in it, so it could sit unchanged for an
hour while you were moving up. The screen says "In the queue" and nothing more,
on purpose.

---

## Scheduling and wipe day

### I cannot join because a join is scheduled
One PC does one thing at a time, and a scheduled join reserves it. Joining now
would cost you the scheduled one, usually the wipe. Cancel the schedule on the
Schedule page if you would rather play now.

### How do I set up for a wipe?
Night before: PC green on the dashboard, server saved, join scheduled for a few
minutes **before** the announced wipe time with "wait for the server to come
back up" ticked, PC left on and signed in with Steam running and Rust closed.
The time is your local time wherever you are; the PC acts on the same instant.

### Will it beat everyone else?
It queries the server directly every couple of seconds and connects the moment
it answers, which is faster than a person refreshing the browser. Force wipe
also ships a game update, so Steam has gigabytes to fetch first: QueueUp waits
that out, shows the progress, and connects after.

---

## Account

Everything about the account lives on one page: **Settings**, top right of
every screen.

### I want to change my password
Settings, then **Change password**. It asks for the current one. Doing it signs
out anything else that is signed in to the account, which is how to throw off a
PC or phone you no longer trust. You stay signed in where you changed it.

### I forgot my password
"Forgotten your password?" on the sign-in page emails a link, good for one hour
and one use. Completing it signs the account out everywhere.

### Can I use two PCs?
One account, one PC. To move to a different PC, go to Settings and press
**Unlink** next to it, then pair the new one. Unlinking stops any join running
on that PC and cancels joins scheduled for it.

### Delete my account and data
Settings, then **Delete my account**. It asks for the password and for the word
DELETE.

Nothing is deleted straight away. The account is scheduled for deletion in
**seven days**, and an email says so. Nothing stops working in the meantime:
the PC, schedules and saved servers all carry on as normal. On the day,
everything goes at once: PC, joins and their timelines, schedules, saved
servers, feedback and problem reports. After that it cannot be brought back.

If a subscription is running it has to be cancelled first, under Subscription
on the same page. Otherwise the card would keep being charged with no account
left to cancel from.

### I asked to delete my account and I want to stop it
Sign in. Every screen carries a bar saying when the account goes, with a
**Keep my account** button. Pressing it cancels the deletion completely, with
nothing lost. It works any time before the day.

### I got an email saying my account will be deleted and I did not ask
Somebody else is in the account. Sign in and press **Keep my account** on the
bar at the top, then change the password in Settings, which signs out whoever
else is signed in.

---

## Trust questions people ask before installing

### Will this get me banned?
No. QueueUp does four things and nothing else: opens the game through Steam the
way a bookmark would, reads the game's own log file, checks whether the game is
running, and closes it when you say so. No memory reading, no injection, no
simulated keys or clicks, no touching game files. In game, everything is still
you.

### Do you need my Steam password?
Never. There is nowhere to type one. Steam stays signed in on your own PC
exactly as it is now.

### What do you keep about me?
Email, your PC's name and version, and what your joins did. Problem reports you
choose to send contain QueueUp's log and the end of Rust's log, which includes
your Steam ID because Rust writes it there. Full detail on the privacy page.

---

## Sending a problem report

Right-click the QueueUp icon by the clock (check under the **^** arrow) →
**Send a problem report to QueueUp**. It goes straight to us. Then say what
happened on the feedback page, and roughly when.

If it went wrong **in the game**, send the report before starting Rust again:
the game clears its log every time it starts.

Older versions only have "Save a problem report", which puts a file on the
Desktop instead. QueueUp updates itself, so that fixes itself.

---

## Known limits, so nobody is promised what does not exist

- Windows only, because Rust is.
- The PC must be awake and signed in, with Steam running. QueueUp does not wake
  sleeping PCs and cannot turn a PC on.
- One PC per account.
- No clan or group joins yet.
- QueueUp cannot make a server let you in: it queues the normal way, like you.
