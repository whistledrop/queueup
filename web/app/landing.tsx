import Link from 'next/link'
import { BETA, PLAN, costLine } from '@/lib/pricing'
import s from './landing.module.css'

// The landing page. Everything on it is a picture of the real app: the phone
// mockups are the actual screens, rebuilt in markup so they stay pin sharp.

export default function Landing() {
  return (
    <div className={s.page}>
      <nav className={s.nav}>
        <span className="brand">
          Queue<span>Up</span>
          {BETA && <span className="beta">beta</span>}
        </span>
        <span style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
          <Link href="/help" className={`${s.signin} ${s.navHide}`}>Help</Link>
          <Link href="/login" className={s.signin}>Sign in</Link>
          <Link href="/login?mode=create" className={s.navCta}>Get QueueUp</Link>
        </span>
      </nav>

      <header className={s.hero}>
        <div>
          <h1>
            Join Rust from your phone.{' '}
            <em>Your PC sits in the queue.</em>
          </h1>
          <p className={s.lede}>
            Tap join from school, work or the traffic. Walk in and play.
          </p>
          <ul className={s.heroPoints}>
            <li>Running late for wipe? Start queueing before you leave.</li>
            <li>No 20 minute load screen. You are already in.</li>
            <li>Not a cheat. It never touches the game.</li>
          </ul>
          <div className={s.ctaRow}>
            <Link href="/login?mode=create" className={s.cta}>
              Get QueueUp free
            </Link>
            <span className={s.ctaNote}>{costLine()}</span>
          </div>
        </div>
        <div>
          <LivePhone />
        </div>
      </header>

      <section className={s.section}>
        <p className={s.kicker}>How it works</p>
        <h2>Three steps, then never again</h2>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>1</span>
            <h3>Link your PC</h3>
            <p>One file, one six character code. No router settings, ever.</p>
          </div>
          <div className={s.stepVisual}>
            <PairVisual />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>2</span>
            <h3>Tap join, from anywhere</h3>
            <p>Search any server. Your phone tells your PC. That is it.</p>
          </div>
          <div className={s.stepVisual}>
            <ServersPhone />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>3</span>
            <h3>Turn up and play</h3>
            <p>
              It queues, loads the map and holds your slot. Crash or reboot, it
              rejoins on its own.
            </p>
          </div>
          <div className={s.stepVisual}>
            <PcVisual />
          </div>
        </div>

        <Diagram />
        <p className={s.diagramCaption}>
          Nothing connects in to your PC. It calls out, like a chat app.
        </p>
      </section>

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
      </section>

      <section className={s.section} id="pricing">
        <p className={s.kicker}>Price</p>
        <h2>Free while it is in beta</h2>
        <div className={s.priceCard}>
          <div className={s.priceAmount}>
            {BETA ? (
              <>
                Free<small> during the beta</small>
              </>
            ) : (
              <>
                {PLAN.symbol}
                {PLAN.monthly.toFixed(2)}
                <small> / month</small>
              </>
            )}
          </div>
          <ul className={s.priceIncludes}>
            {PLAN.includes.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
          <Link href="/login?mode=create" className={s.cta} style={{ display: 'block' }}>
            Get QueueUp free
          </Link>
          <p className={s.priceNote}>
            {BETA
              ? `All we ask is that you say how it went. ${PLAN.symbol}${PLAN.monthly.toFixed(2)} a month when the beta ends, and nobody is charged without subscribing.`
              : 'Setting up is free. You pay when you first join a server, and you can cancel anytime.'}
          </p>
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
            <p>
              Two minutes on the PC. Windows will warn it does not recognise the
              app, because it is not signed yet: choose More info, then Run
              anyway.
            </p>
          </details>
        </div>
      </section>

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
  )
}

/* ------------------------------------------------------------ visuals */

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
        <div className={s.mockCard}>
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
          <path d="M0,0 L8,4 L0,8 z" fill="#99a2ab" />
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
      <line x1="132" y1="75" x2="226" y2="75" stroke="#99a2ab" strokeWidth="1.5" markerEnd="url(#arr)" />
      <text x="179" y="65" textAnchor="middle" fill="#99a2ab" fontSize="11">
        tap join
      </text>

      <line x1="448" y1="75" x2="354" y2="75" stroke="#99a2ab" strokeWidth="1.5" markerEnd="url(#arr)" />
      <text x="401" y="65" textAnchor="middle" fill="#99a2ab" fontSize="11">
        PC connects out
      </text>

      <line x1="562" y1="75" x2="636" y2="75" stroke="#99a2ab" strokeWidth="1.5" markerEnd="url(#arr)" />
      <text x="599" y="65" textAnchor="middle" fill="#99a2ab" fontSize="11">
        Steam
      </text>
    </svg>
  )
}
