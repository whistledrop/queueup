import Link from 'next/link'
import { PLAN, priceLine } from '@/lib/pricing'
import s from './landing.module.css'

// The landing page. Everything on it is a picture of the real app: the phone
// mockups are the actual screens, rebuilt in markup so they stay pin sharp.

export default function Landing() {
  return (
    <div className={s.page}>
      <nav className={s.nav}>
        <span className="brand">
          Queue<span>Up</span>
        </span>
        <Link href="/login" className={s.signin}>
          Sign in
        </Link>
      </nav>

      <header className={s.hero}>
        <div>
          <h1>
            Your PC waits in the Rust queue. <em>You get on with your day.</em>
          </h1>
          <p className={s.lede}>
            Tap join on your phone. At home, your PC launches Rust, sits through
            the queue and holds your slot until you sit down. Built for wipe day.
          </p>
          <div className={s.ctaRow}>
            <Link href="/login?mode=create" className={s.cta}>
              Get QueueUp
            </Link>
            <span className={s.ctaNote}>
              {priceLine()}. Setting up is free, you pay when you first join.
            </span>
          </div>
        </div>
        <div>
          <LivePhone />
        </div>
      </header>

      <section className={s.section}>
        <p className={s.kicker}>How it works</p>
        <h2>Exactly what happens, step by step</h2>
        <p className={s.sectionIntro}>
          No magic and no tricks. One small program on your PC, one website on
          your phone, and the game&apos;s own queue doing what it always does.
        </p>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>1</span>
            <h3>Link your PC, once</h3>
            <p>
              Download one small file onto your gaming PC and double-click it.
              It shows a six character code. Type that code into the website on
              your phone and the two are paired. The agent then sits in your
              system tray, connected out to QueueUp, waiting. There is nothing
              to configure and no router settings to change.
            </p>
          </div>
          <div className={s.stepVisual}>
            <PairVisual />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>2</span>
            <h3>Pick a server, tap join</h3>
            <p>
              Search any Rust server by name, save your regulars, and tap Join
              from wherever you are. The command reaches your PC through a
              connection your PC opened itself, so there is no router setup and
              no port forwarding, ever.
            </p>
          </div>
          <div className={s.stepVisual}>
            <ServersPhone />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>3</span>
            <h3>Your PC does the waiting</h3>
            <p>
              It starts Rust through Steam, pointed at your server. Rust joins
              the queue the way it always does. The agent follows progress by
              reading the game&apos;s own log file, and if Rust crashes mid
              queue it relaunches and rejoins on its own.
            </p>
          </div>
          <div className={s.stepVisual}>
            <PcVisual />
          </div>
        </div>

        <div className={s.step}>
          <div>
            <span className={s.stepNumber}>4</span>
            <h3>Watch it live, get told when it matters</h3>
            <p>
              Your queue position updates live on your phone. Notifications at
              100, 50 and 10 places to go, one when you are in, and one if your
              PC ever drops offline. The slot is held until you get there.
            </p>
          </div>
          <div className={s.stepVisual}>
            <LivePhone />
          </div>
        </div>

        <Diagram />
        <p className={s.diagramCaption}>
          Nothing ever connects in to your PC. It makes one outbound connection
          and holds it open, like a chat app.
        </p>
      </section>

      <section className={s.section}>
        <p className={s.kicker}>Wipe day</p>
        <h2>Schedule it, then watch the restart get beaten</h2>
        <p className={s.sectionIntro}>
          Set the join for a few minutes before the announced wipe. When the
          server goes down for its restart, QueueUp queries it directly, every
          couple of seconds, and connects the moment it answers again. Faster
          than anyone mashing refresh in the server browser. If Windows forces
          a reboot mid queue, the job survives that too: your PC reconnects and
          picks it straight back up.
        </p>
        <div className={s.stepVisual}>
          <SchedulePhone />
        </div>
      </section>

      <section className={s.section}>
        <p className={s.kicker}>Fair play</p>
        <h2>Not a cheat. Not even close.</h2>
        <p className={s.sectionIntro}>
          Rust runs Easy Anti-Cheat, and QueueUp is built to stay far away from
          it. The agent is allowed to do exactly four things, and all four are
          things you could do yourself with a mouse.
        </p>
        <div className={s.fairGrid}>
          <div className={s.fairItem}>
            <h4>Starts the game through Steam</h4>
            <p>The same steam link a browser bookmark would use. Nothing more.</p>
          </div>
          <div className={s.fairItem}>
            <h4>Reads the game&apos;s log file</h4>
            <p>
              A text file Rust writes anyway. That is how it knows your queue
              position.
            </p>
          </div>
          <div className={s.fairItem}>
            <h4>Checks the game is running</h4>
            <p>So it can relaunch after a crash. It never touches the process.</p>
          </div>
          <div className={s.fairItem}>
            <h4>Closes it when you say so</h4>
            <p>The cancel button on your phone, nothing else.</p>
          </div>
        </div>
        <p className={s.sectionIntro} style={{ marginTop: 28, marginBottom: 0 }}>
          No memory reading, no injection, no simulated keys or clicks, no
          touching game files. QueueUp never asks for your Steam password and
          has no way to use it. In game, everything is still you.
        </p>
      </section>

      <section className={s.section} id="pricing">
        <p className={s.kicker}>Price</p>
        <h2>One plan, no tiers</h2>
        <p className={s.sectionIntro}>
          Less than the cost of losing one wipe night to a queue you were not
          even home for.
        </p>
        <div className={s.priceCard}>
          <div className={s.priceAmount}>
            {PLAN.symbol}
            {PLAN.monthly.toFixed(2)}
            <small> / month</small>
          </div>
          <ul className={s.priceIncludes}>
            {PLAN.includes.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
          <Link href="/login?mode=create" className={`${s.cta}`} style={{ display: 'block' }}>
            Create your account
          </Link>
          <p className={s.priceNote}>
            Setting up is free: account, linking your PC, all of it. You pay
            when you first join a server, and you can cancel anytime. Payments
            are not switched on yet, so early accounts run free until they are.
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
              QueueUp does nothing a ban system looks for. It starts the game
              through Steam, reads a log file, and that is the whole
              relationship. The queue it waits in is Rust&apos;s own queue,
              joined the normal way. It never plays, moves, or acts in game for
              you.
            </p>
          </details>
          <details>
            <summary>Does my PC have to stay on?</summary>
            <p>
              Yes. QueueUp assumes an always on PC: no sleep, no hibernate. It
              does not wake sleeping machines. What it does handle is restarts:
              if Windows forces an update reboot mid queue, the agent starts
              back up, reconnects and resumes the join on its own.
            </p>
          </details>
          <details>
            <summary>Do you need my Steam password?</summary>
            <p>
              No, never. Steam stays signed in on your own PC exactly as it is
              now. QueueUp has no field to type a Steam password into.
            </p>
          </details>
          <details>
            <summary>Which servers does it work with?</summary>
            <p>
              Any Rust server you can see in the server browser, official or
              community. You search by name and QueueUp keeps track of the
              address, even when a server moves between wipes.
            </p>
          </details>
          <details>
            <summary>What do I need to set up?</summary>
            <p>
              About two minutes. On your gaming PC, download the QueueUp agent
              from your dashboard and double-click it. It shows a code, you type
              that code into the website, and the PC is linked. Windows will warn
              that it does not recognise the app, because it is not code signed
              yet: choose More info, then Run anyway. After that, the only other
              things are settings any always on gaming PC wants anyway: sleep
              off, and Steam set to start with Windows.
            </p>
          </details>
          <details>
            <summary>Does it work on Mac?</summary>
            <p>
              The agent is Windows only, because Rust is. You can control it from
              any phone, tablet or computer: the website works everywhere.
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
          <div className={s.mockPosition}>47</div>
          <div className={s.mockSub}>place in the queue</div>
        </div>
        <div className={s.mockCard}>
          <div className={s.mockLabel}>What happened</div>
          <ul className={s.mockTimeline}>
            <li>Launching Rust</li>
            <li>In queue, position 212</li>
            <li>In queue, position 108</li>
            <li>In queue, position 47</li>
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
          <div className={s.mockLabel}>Notifications</div>
          <div className={s.mockSub}>On. Test sent to this phone.</div>
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
        <strong>in queue, position 212</strong>
        <br />
        in queue, position 108
        <br />
        in queue, position 47
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
