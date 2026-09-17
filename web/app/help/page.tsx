import Link from 'next/link'
import { Footer } from '../nav'
import s from './help.module.css'

// The help page.
//
// Organised by SYMPTOM, not by feature, because that is how somebody arrives:
// they do not know which part failed, they know what they can see. Every answer
// names the thing on their screen and says what to do about it, and none of it
// assumes they know what an agent, a relay or a log file is.

export const metadata = {
  title: 'Help - QueueUp',
  description: 'What to do when something is not working.',
}

function Q({ q, children }: { q: string; children: React.ReactNode }) {
  return (
    <details className={s.q}>
      <summary>{q}</summary>
      <div className={s.a}>{children}</div>
    </details>
  )
}

export default function HelpPage() {
  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn quiet">Back</Link>
      </header>

      <h1 className={s.h1}>Help</h1>
      <p className="muted">
        Find what you can see on your screen. If none of it fits, send a problem
        report (bottom of this page) and <Link href="/feedback">tell us</Link>{' '}
        what happened.
      </p>

      <div className="card">
        <h2>Getting started</h2>

        <Q q="How do I set it up?">
          <ol>
            <li>Create an account here on your phone.</li>
            <li>
              On your gaming PC, open this website and press Download. Windows
              will warn you about the file because it is not signed by a company
              yet: choose <b>More info</b>, then <b>Run anyway</b>.
            </li>
            <li>
              Put the file somewhere permanent (not your Downloads folder) and
              double-click it. A QueueUp icon appears by the clock.
            </li>
            <li>
              It shows a six character code. Type that code into this website on
              your phone. That links the two.
            </li>
            <li>
            <b>Stop the PC going to sleep.</b> On the PC: Settings, System,
            Power, set <b>&quot;When plugged in, put my device to sleep
            after&quot;</b> to <b>Never</b>. This one matters more than it
            sounds: a sleeping PC cannot join anything and QueueUp cannot wake
            it, so a join scheduled for hours later is simply missed. Letting the
            screen turn off is fine.
          </li>
          </ol>
          <p>
            That is it. QueueUp starts with Windows from then on and keeps itself
            up to date. It checks the sleep setting too, and says so on your
            dashboard if it would cause trouble.
          </p>
        </Q>

        <Q q="What does QueueUp actually do to my game?">
          <p>
            Four things, and nothing else: it starts Rust through Steam the same
            way clicking a link does, it reads the game&apos;s own log file to see
            what is happening, it checks whether the game is running, and it can
            close the game the way you would.
          </p>
          <p>
            It never touches the game&apos;s files or memory, and never presses
            keys for you. Rust runs Easy Anti-Cheat and QueueUp is built to stay
            well clear of it.
          </p>
        </Q>

        <Q q="Do you need my Steam password?">
          <p>
            No. QueueUp never asks for it, never stores it, and has no way to use
            it. Steam stays signed in on your own PC exactly as it is now.
          </p>
        </Q>

        <Q q="Does my PC have to stay on?">
          <p>
            Yes. QueueUp does the waiting on your PC, so the PC has to be awake
            and signed in to Windows, with Steam running. Turn off sleep in
            Windows power settings, or it will nod off and miss the wipe.
          </p>
        </Q>
      </div>

      <div className="card">
        <h2>Something is wrong</h2>

        <Q q="It says my PC is offline, but the PC is switched on">
          <p>Work down this list:</p>
          <ol>
            <li>
              <b>Is the QueueUp icon by the clock?</b> Click the small{' '}
              <b>^</b> arrow next to the clock: Windows hides new icons there.
              No icon means QueueUp is not running. Double-click the QueueUp file
              to start it.
            </li>
            <li>
              <b>Is the PC actually awake?</b> A sleeping PC looks identical to a
              switched-off one from here.
            </li>
            <li>
              <b>Is the PC online?</b> Open any website on it.
            </li>
            <li>
              Right-click the QueueUp icon and check it does not say &quot;Stopped&quot;.
              If it does, quit it and start it again.
            </li>
          </ol>
          <p>
            The dot on your dashboard goes green within a few seconds of QueueUp
            starting.
          </p>
        </Q>

        <Q q="It is stuck on Connecting and never reaches the queue">
          <p>
            Connecting can genuinely take a couple of minutes: Rust and Easy
            Anti-Cheat are slow to start, and the map has to load.
          </p>
          <p>If it sits there much longer:</p>
          <ul>
            <li>
              Look at the PC. If Rust is showing an error or a Steam window, that
              is your answer.
            </li>
            <li>
              If Rust is not running at all, Steam may be mid-update. Your phone
              says so when that is happening.
            </li>
            <li>
              Cancel the join and tap Join again. If it happens twice, save a
              problem report.
            </li>
          </ul>
        </Q>

        <Q q="Steam is updating Rust">
          <p>
            Normal, especially on force wipe day: Rust ships an update with the
            wipe and Steam has to download several gigabytes. QueueUp waits for
            it and connects as soon as it finishes. There is nothing to do.
          </p>
          <p>
            Two things make it quicker, both worth doing the night before: in
            Steam, right-click Rust, Properties, Updates, set{' '}
            <b>Always keep this game updated</b>; and leave Steam running,
            because a closed Steam downloads nothing.
          </p>
        </Q>

        <Q q="Steam has paused, or stopped, downloading Rust">
          <p>
            This one needs you at the PC: waiting will not fix it. Open Steam and
            look at Downloads. Usually it is one of:
          </p>
          <ul>
            <li>the download was paused, so press resume;</li>
            <li>
              Steam is in offline mode (Steam menu, then <b>Go Online</b>);
            </li>
            <li>the drive is full. Rust wipes need several gigabytes free.</li>
          </ul>
          <p>QueueUp carries on by itself once the download moves again.</p>
        </Q>

        <Q q="It says Steam is not logged in">
          <p>
            Sign in to Steam on the PC and start the join again. QueueUp tries a
            couple of times before saying this, so a Steam that was merely
            restarting will have sorted itself out already.
          </p>
        </Q>

        <Q q="Rust closed on its own, or QueueUp keeps reopening it">
          <p>
            If Rust <b>crashed</b>, QueueUp relaunches it and rejoins the queue
            for you. That is deliberate, and your phone says &quot;Rust closed
            unexpectedly&quot;.
          </p>
          <p>
            If <b>you</b> closed Rust, QueueUp takes that as a cancel and stops.
            It does not reopen a game a person just closed. If it did reopen
            after you closed it, something is wrong: send a problem report.
          </p>
        </Q>

        <Q q="I got in, then QueueUp closed my game">
          <p>
            It should never do that. Once you are in a server the job is finished
            and QueueUp leaves the game completely alone, including if you quit
            the QueueUp icon. If this happened, send a problem report from
            the tray icon: that is a bug and worth knowing about immediately.
          </p>
        </Q>

        <Q q="Why doesn't it show my place in the queue?">
          <p>
            Because nothing outside the game knows it. Rust does not write your
            queue position to its log, and it does not publish it anywhere else
            either. QueueUp used to show a number worked out from how long the
            server&apos;s whole queue was, and it was misleading: that is the
            length of the line, not your place in it, so it could sit unchanged
            for an hour while you were steadily moving up.
          </p>
          <p>
            So the screen now says simply that you are in the queue. When you get
            through it changes to loading into the server, then &quot;You&apos;re
            in&quot;, and those are real: they come from the game&apos;s own log.
          </p>
        </Q>

        <Q q="I left the server and the app is still showing the join">
          <p>
            Leaving the server, or backing out of the queue, stops the join. So
            does closing Rust. QueueUp reads that from the game&apos;s log and
            finishes the join as cancelled, and it will not relaunch the game or
            pull you back into the server you just left.
          </p>
          <p>
            If the app kept showing the join running for more than a minute after
            you left, that is worth reporting: send a problem report from the
            tray icon. The exact wording Rust uses when you
            disconnect varies between builds, and the report contains the lines
            needed to fix it.
          </p>
        </Q>

        <Q q="It says the server refused the connection">
          <p>
            That came from the server, not from QueueUp: usually a password, a
            whitelist, or a ban. Check you can join that server by hand before
            trying again.
          </p>
        </Q>

        <Q q="It tried a few times and gave up">
          <p>
            QueueUp retries a bounded number of times, waiting longer each time,
            then stops rather than hammering a server forever. Look at the
            timeline on the join screen: the reason it gave up is in there. Fix
            that, then tap Join again.
          </p>
        </Q>

        <Q q="Nothing on my phone is updating">
          <p>
            The live screen refreshes itself every couple of seconds while a join
            is running. If it has frozen, pull down to refresh the page. If the
            whole site fails to load, the relay may be restarting: wait a minute
            and try again.
          </p>
        </Q>
      </div>

      <div className="card">
        <h2>Scheduled joins and wipe day</h2>

        <Q q="How do I set up a wipe day join?">
          <p>
            Go to Schedule, pick your server and the wipe time, and tick{' '}
            <b>join as soon as the server comes back up</b>. QueueUp then watches
            that server, and the moment it returns from its wipe restart your PC
            starts connecting, without waiting for the clock.
          </p>
          <p>
            Times are in your phone&apos;s local time. If you are travelling, set
            it in the time zone you are actually in and it will be right.
          </p>
        </Q>

        <Q q="What happens if my PC is off when a scheduled join fires?">
          <p>
            The join is saved and starts the moment your PC comes back. Nothing
            is lost. The timeline on the join will say the PC was offline.
          </p>
        </Q>

        <Q q="What if the PC restarts in the middle of a queue?">
          <p>
            QueueUp starts again with Windows, reconnects, picks the job back up
            and requeues by itself. You lose your place in the queue, because the
            game did, but nothing needs doing at your end.
          </p>
        </Q>
      </div>

      <div className="card">
        <h2>Still stuck</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Send a problem report. It takes one click and tells us exactly what
          happened.
        </p>
        <ol>
          <li>
            On the PC, right-click the QueueUp icon by the clock (check under the{' '}
            <b>^</b> arrow).
          </li>
          <li>
            Choose <b>Send a problem report to QueueUp</b>. The icon says
            &quot;Problem report sent&quot; when it has gone.
          </li>
          <li>
            Then <Link href="/feedback">tell us what happened</Link> in a
            sentence or two, and roughly what time.
          </li>
        </ol>
        <p className="muted small">
          If the PC is offline, the report is saved to the Desktop instead.
          Older versions of QueueUp only have <b>Save a problem report</b>: use
          that, and QueueUp will update itself soon.
        </p>
        <p className="muted small">
          The report holds the QueueUp log and the most recent part of the
          game&apos;s log. No passwords. Rust writes your Steam ID into its log,
          so the report includes that. See <Link href="/privacy">privacy</Link>.
        </p>
        <p className="muted small">
          If it went wrong in the game, send the report{' '}
          <b>before starting Rust again</b>: the game clears its log every time
          it starts.
        </p>
      </div>

      <Footer />
    </div>
  )
}
