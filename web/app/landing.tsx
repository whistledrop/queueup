import Link from 'next/link'
import { BETA } from '@/lib/pricing'
import s from './landing.module.css'
import StartForm from './startForm'
import HeroShot, { Spawn } from './heroShot'
import Rotator from './rotator'
import { CtaRow, StickyCta } from './ctas'
import Testimonials from './testimonials'
import Demo from './demo'

// The landing page. Everything on it is a picture of the real app: the phone
// mockups are the actual screens, rebuilt in markup so they stay pin sharp.

export default function Landing() {
  return (
    <div className={s.light} data-landing="">
    <div className={s.page}>
      <nav className={s.nav}>
        <span className="brand">
          Queue<span>Up</span>
          {BETA && <span className="beta">beta</span>}
        </span>
        {/* Only Sign in. Help belonged to somebody already using QueueUp and
            stuck; on the front door it offered a stranger a manual for a thing
            they have not bought, and took the eye off the one thing this page
            is for. It is still in the app, where being stuck happens. */}
        <Link href="/login" className={s.signin}>Sign in</Link>
      </nav>

      <header className={s.hero}>
        <div>
          <h1>
            Load into Rust servers from <Rotator />
          </h1>
          <p className={s.lede}>
            Tap join from wherever you are. Your PC queues. You walk in and play.
          </p>
          <StartForm className={s.startForm} />
        </div>
        <div>
          <HeroShot />
        </div>
      </header>

      <Testimonials />

      <section className={s.section} id="how">
        <p className={s.kicker}>How it works</p>
        <h2>Three steps, then never again</h2>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>1</span>
            <h3>Tap join from anywhere</h3>
            <p>Search any server, official or community, and press one button.</p>
          </div>
          <div className={s.stepVisual}>
            <ServersPhone />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>2</span>
            <h3>Your PC queues and loads in</h3>
            <p>
              It waits in the queue, loads the map and holds your slot. Crash or
              reboot, it rejoins on its own.
            </p>
          </div>
          <div className={s.stepVisual}>
            <PcVisual />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>3</span>
            <h3>Walk in and play</h3>
            <p>Sit down to a game that is already running, already in.</p>
          </div>
          <div className={s.stepVisual}>
            {/* The game, loaded in: the beach you wake up on. Not the pairing
                code, which is a step they do once and never think about. */}
            <div className={s.spawnFrame}>
              <Spawn />
            </div>
          </div>
        </div>

        <p className={s.setupNote}>
          First time only: a 2-minute install on your PC, like pairing a speaker.
        </p>

        <Diagram />
        <p className={s.diagramCaption}>
          Nothing connects in to your PC. It calls out, like a chat app.
        </p>
        <CtaRow where="how_it_works" />
      </section>

      <Demo />

      <section className={s.section}>
        <p className={s.kicker}>Wipe day</p>
        <h2>Beat the restart</h2>
        <p className={s.sectionIntro}>
          Schedule it before you leave. QueueUp pings the server every couple of
          seconds and connects the moment it is back, game update and all.
        </p>
        <div className={s.stepVisual}>
          <SchedulePhone />
        </div>
      </section>

      <section className={s.section}>
        <p className={s.kicker}>Fair play</p>
        <h2>Not a cheat. Not even close.</h2>
        <p className={s.sectionIntro}>
          Four things, all of them things you could do with a mouse.
        </p>
        <div className={s.fairGrid}>
          <div className={s.fairItem}>
            <h4>Opens the game through Steam</h4>
            <p>The same link a bookmark would use.</p>
          </div>
          <div className={s.fairItem}>
            <h4>Reads the game&apos;s log file</h4>
            <p>A text file Rust writes anyway.</p>
          </div>
          <div className={s.fairItem}>
            <h4>Checks the game is running</h4>
            <p>So it can relaunch after a crash.</p>
          </div>
          <div className={s.fairItem}>
            <h4>Closes it when you say</h4>
            <p>Your cancel button, nothing else.</p>
          </div>
        </div>
        <p className={s.diagramCaption} style={{ marginTop: 18 }}>
          No memory reading, no key presses, no game files. In game it is all
          still you.
        </p>
        <CtaRow where="fair_play" />
      </section>

      {/* The same ask as the top of the page: the offer, and a box for the
          address. Somebody who has read this far has decided, and should be
          able to act without scrolling back up. */}
      <section className={s.section} id="pricing">
        <div className={s.claimBlock}>
          <StartForm className={s.startForm} from="pricing" />
        </div>
      </section>

      <section className={s.section}>
        <p className={s.kicker}>Questions</p>
        <h2>The ones everyone asks</h2>
        <div className={s.faq}>
          <details>
            <summary>Will this get me banned?</summary>
            <p>
              No. It opens the game through Steam and reads a log file. It never
              plays, moves or acts in game for you.
            </p>
          </details>
          <details>
            <summary>Does my PC have to stay on?</summary>
            <p>
              Yes, awake and signed in, with Steam running. It cannot wake a
              sleeping PC, but it does survive a Windows reboot.
            </p>
          </details>
          <details>
            <summary>Do you need my Steam password?</summary>
            <p>Never. There is nowhere to type one.</p>
          </details>
          <details>
            <summary>Which servers work?</summary>
            <p>
              Any Rust server in the browser, official or community. Search by
              name and QueueUp follows the address between wipes.
            </p>
          </details>
          <details>
            <summary>How long does setup take?</summary>
            <p>Two minutes, once.</p>
          </details>
        </div>
      </section>

      <StickyCta />

      <footer className={s.footer}>
        <p>
          QueueUp is an unofficial third party tool and is not affiliated with,
          endorsed by, or connected to Facepunch Studios. It never modifies or
          automates the game itself. Rust is a trademark of Facepunch Studios.
        </p>
        <p>
          <Link href="/login">Sign in</Link>
        </p>
      </footer>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------ visuals */

// The hero picture.
//
// A phone showing one status card said what the app looks like, not what it
// does, and what it does is the entire pitch: the tap happens on the thing in
// your hand, and the work happens on a machine that is somewhere else. One
// screen cannot show that, because the whole point is that there are two of
// them and they are in different buildings.
//
// So: your thumb on Join, in front of your PC getting on with it. Cause on the
// left, effect behind it, and a label on each saying where it is.
function LivePhone() {
  return (
    <div className={s.phone}>
      <div className={s.phoneNotch} />
      <div className={s.screen}>
        <div className={s.screenBrand}>
          Queue<span>Up</span>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockLabel}>Rustopia EU Main</div>
          <div className={s.mockState}>In the queue</div>
          <div className={s.mockSub}>your PC is waiting to get in</div>
        </div>
        {/* The timeline is the nice-to-have half of the shot. On a short
            screen it is the difference between the button being visible and
            not, and a button below the fold costs more than a detail. */}
        <div className={`${s.mockCard} ${s.whenTall}`}>
          <div className={s.mockLabel}>What happened</div>
          <ul className={s.mockTimeline}>
            <li>Launching Rust</li>
            <li>Connecting to the server</li>
            <li>In the queue</li>
            <li>Loading into the server</li>
          </ul>
        </div>
      </div>
    </div>
  )
}

function ServersPhone() {
  return (
    <div className={s.phone}>
      <div className={s.phoneNotch} />
      <div className={s.screen}>
        <div className={s.screenBrand}>
          Queue<span>Up</span>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockInput}>rustopia</div>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockRow}>
            <div>
              <div className={s.mockName}>Rustopia EU Main</div>
              <div className={s.mockSub}>198 / 200 players, 312 in queue</div>
            </div>
            <span className={s.mockBtn}>Join</span>
          </div>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockRow}>
            <div>
              <div className={s.mockName}>Rustopia EU Barren</div>
              <div className={s.mockSub}>112 / 150 players</div>
            </div>
            <span className={s.mockBtn}>Join</span>
          </div>
        </div>
      </div>
    </div>
  )
}

function SchedulePhone() {
  return (
    <div className={s.phone}>
      <div className={s.phoneNotch} />
      <div className={s.screen}>
        <div className={s.screenBrand}>
          Queue<span>Up</span>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockLabel}>Your PC</div>
          <div className={s.mockRow}>
            <div className={s.mockName}>
              <span className={s.mockDotOn} />
              Gaming PC
            </div>
            <span className={s.mockSub}>Online and ready</span>
          </div>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockLabel}>Scheduled joins</div>
          <div className={s.mockRow}>
            <div>
              <div className={s.mockName}>Rustopia EU Main</div>
              <div className={s.mockSub}>
                Thu 19:55, waits for the wipe restart
              </div>
            </div>
            <span className={s.mockGhostBtn}>Cancel</span>
          </div>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockLabel}>Sleep</div>
          <div className={s.mockSub}>Off. This PC will stay awake for it.</div>
        </div>
      </div>
    </div>
  )
}

function PairVisual() {
  return (
    <div className={s.pcWindow}>
      <div className={s.pcTitlebar}>
        <span className={s.pcDot} />
        <span className={s.pcDot} />
        <span className={s.pcDot} />
        <span style={{ marginLeft: 6 }}>QueueUp agent, on your PC</span>
      </div>
      <div className={s.pcBody}>
        Type this code into the QueueUp
        <br />
        web app:
        <div className={s.mockCode}>E 6 Y 4 X D</div>
        It expires in 10 minutes.
        <br />
        <strong>Waiting...</strong>
      </div>
    </div>
  )
}

function PcVisual() {
  return (
    <div className={s.pcWindow}>
      <div className={s.pcTitlebar}>
        <span className={s.pcDot} />
        <span className={s.pcDot} />
        <span className={s.pcDot} />
        <span style={{ marginLeft: 6 }}>QueueUp agent, on your PC</span>
      </div>
      <div className={s.pcBody}>
        job received: Rustopia EU Main
        <br />
        launching Rust via Steam
        <br />
        connecting to 51.83.128.10
        <br />
        <strong>in the queue, waiting</strong>
        <br />
        through the queue, loading world
        <br />
        spawned in, holding the slot
      </div>
    </div>
  )
}

function Diagram() {
  return (
    <svg
      className={s.diagram}
      viewBox="0 0 720 150"
      width="720"
      role="img"
      aria-label="Your phone talks to QueueUp, QueueUp talks to your PC over a connection the PC opened, and your PC talks to the Rust server."
    >
      <defs>
        <marker id="arr" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">
          <path d="M0,0 L8,4 L0,8 z" fill="#7a838d" />
        </marker>
      </defs>

      {/* phone */}
      <rect x="20" y="35" width="110" height="80" rx="14" fill="#191c1f" stroke="#2b3036" />
      <rect x="55" y="43" width="40" height="5" rx="2.5" fill="#2b3036" />
      <text x="75" y="82" textAnchor="middle" fill="#eef1f4" fontSize="14" fontWeight="600">
        Your phone
      </text>
      <text x="75" y="100" textAnchor="middle" fill="#99a2ab" fontSize="11">
        anywhere
      </text>

      {/* relay */}
      <rect x="230" y="35" width="120" height="80" rx="14" fill="#191c1f" stroke="#d05a2a" />
      <text x="290" y="72" textAnchor="middle" fill="#eef1f4" fontSize="14" fontWeight="600">
        QueueUp
      </text>
      <text x="290" y="90" textAnchor="middle" fill="#99a2ab" fontSize="11">
        remembers everything
      </text>

      {/* pc */}
      <rect x="450" y="35" width="110" height="80" rx="14" fill="#191c1f" stroke="#2b3036" />
      <text x="505" y="72" textAnchor="middle" fill="#eef1f4" fontSize="14" fontWeight="600">
        Your PC
      </text>
      <text x="505" y="90" textAnchor="middle" fill="#99a2ab" fontSize="11">
        at home, on
      </text>

      {/* server */}
      <rect x="640" y="35" width="60" height="80" rx="10" fill="#191c1f" stroke="#2b3036" />
      <circle cx="670" cy="58" r="4" fill="#4caf7d" />
      <text x="670" y="82" textAnchor="middle" fill="#eef1f4" fontSize="12" fontWeight="600">
        Rust
      </text>
      <text x="670" y="98" textAnchor="middle" fill="#99a2ab" fontSize="10">
        server
      </text>

      {/* arrows */}
      <line x1="132" y1="75" x2="226" y2="75" stroke="#7a838d" strokeWidth="1.5" markerEnd="url(#arr)" />
      <text x="179" y="65" textAnchor="middle" fill="#5c6672" fontSize="11">
        tap join
      </text>

      <line x1="448" y1="75" x2="354" y2="75" stroke="#7a838d" strokeWidth="1.5" markerEnd="url(#arr)" />
      <text x="401" y="65" textAnchor="middle" fill="#5c6672" fontSize="11">
        PC connects out
      </text>

      <line x1="562" y1="75" x2="636" y2="75" stroke="#7a838d" strokeWidth="1.5" markerEnd="url(#arr)" />
      <text x="599" y="65" textAnchor="middle" fill="#5c6672" fontSize="11">
        Steam
      </text>
    </svg>
  )
}
