'use client'

// The hero picture, playing.
//
// A still of a phone next to a monitor says the two things exist. It does not
// say that pressing one makes the other work, which is the entire product. So
// it runs: the Join button is pressed, the PC starts Rust, the queue counts
// down, and the phone says "You're in" at the same moment the PC does.
//
// It is ten seconds because that is how long the real thing takes to explain,
// and it loops because somebody who arrives halfway through should still get
// the whole story within one read of the headline.

import { useEffect, useState } from 'react'
import s from './landing.module.css'

type Phase = {
  ms: number
  tap?: boolean
  // What the phone says. Mirrors the real screens.
  phone: { label: string; state: string; sub?: string; done?: boolean; join?: boolean }
  // What the PC has printed so far. Accumulates, like a real log.
  log: string[]
}

const SCRIPT: Phase[] = [
  {
    ms: 1500,
    phone: { label: 'Rustopia EU Main', state: '198 / 200', join: true },
    log: [],
  },
  {
    ms: 650,
    tap: true,
    phone: { label: 'Rustopia EU Main', state: '198 / 200', join: true },
    log: [],
  },
  {
    ms: 1300,
    phone: { label: 'Rustopia EU Main', state: 'Launching Rust' },
    log: ['steam ready', 'launching Rust'],
  },
  {
    ms: 1300,
    phone: { label: 'Rustopia EU Main', state: 'Connecting' },
    log: ['steam ready', 'launching Rust', 'connecting'],
  },
  {
    ms: 1400,
    phone: { label: 'Rustopia EU Main', state: 'In the queue', sub: '212 ahead' },
    log: ['steam ready', 'launching Rust', 'connecting', 'in the queue, 212 ahead'],
  },
  {
    ms: 1200,
    phone: { label: 'Rustopia EU Main', state: 'In the queue', sub: '84 ahead' },
    log: ['steam ready', 'launching Rust', 'connecting', 'in the queue, 84 ahead'],
  },
  {
    ms: 1100,
    phone: { label: 'Rustopia EU Main', state: 'In the queue', sub: '9 ahead' },
    log: ['steam ready', 'launching Rust', 'connecting', 'in the queue, 9 ahead'],
  },
  {
    ms: 2600,
    phone: { label: 'Rustopia EU Main', state: "You're in", sub: 'Slot is being held', done: true },
    log: ['steam ready', 'launching Rust', 'connecting', 'through the queue', 'spawned in, holding your slot'],
  },
]

// The frame somebody sees if they have asked for less movement, and the frame
// the page is rendered at before the script starts: the queue, which is the
// one that explains the product best standing still.
const RESTING = 4

export default function HeroShot() {
  const [i, setI] = useState(0)
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      setI(RESTING)
      return
    }
    setPlaying(true)
  }, [])

  useEffect(() => {
    if (!playing) return
    const t = setTimeout(() => setI((n) => (n + 1) % SCRIPT.length), SCRIPT[i].ms)
    return () => clearTimeout(t)
  }, [i, playing])

  const phase = SCRIPT[i]

  return (
    <div className={s.shot}>
      <div className={s.shotSide}>
        <div className={s.iphone}>
          <div className={s.island} />
          <div className={s.iphoneScreen}>
            <div className={s.shotServer}>{phase.phone.label}</div>
            {phase.phone.join ? (
              <>
                <div className={s.shotPop}>{phase.phone.state}</div>
                <div className={`${s.shotJoin} ${phase.tap ? s.shotJoinTap : ''}`}>
                  Join
                  {phase.tap && <span className={s.tapRipple} />}
                </div>
              </>
            ) : (
              <>
                <div className={`${s.shotState} ${phase.phone.done ? s.shotStateDone : ''}`}>
                  {phase.phone.state}
                </div>
                {phase.phone.sub && <div className={s.shotPop}>{phase.phone.sub}</div>}
              </>
            )}
          </div>
          <div className={s.homeBar} />
        </div>
        <p className={s.shotLabel}>Your phone</p>
      </div>

      <svg className={s.shotArrow} viewBox="0 0 34 12" fill="none" aria-hidden="true">
        <path d="M0 6h26" stroke="currentColor" strokeWidth="1.5" strokeDasharray="3 3" />
        <path d="M25 1.5 32 6l-7 4.5z" fill="currentColor" />
      </svg>

      <div className={s.shotSide}>
        <div className={s.monitorWrap}>
          <div className={s.monitor}>
            <div className={s.monitorScreen}>
              <div className={s.pcTitlebar}>
                <span className={s.pcDot} />
                <span className={s.pcDot} />
                <span className={s.pcDot} />
                <span style={{ marginLeft: 6 }}>QueueUp</span>
              </div>
              {/* Five rows always, filled from the top, so the window does not
                  change size as the log grows. */}
              <div className={s.pcBody}>
                {[0, 1, 2, 3, 4].map((row) => {
                  const line = phase.log[row]
                  const last = row === phase.log.length - 1
                  return (
                    <div
                      key={row}
                      className={`${s.pcLine} ${line ? '' : s.pcLineEmpty} ${last ? s.pcLineNow : ''}`}
                    >
                      {line ?? ' '}
                    </div>
                  )
                })}
              </div>
            </div>
          </div>
          <div className={s.stand} />
          <div className={s.base} />
        </div>
        <p className={s.shotLabel}>Your PC, at home</p>
      </div>
    </div>
  )
}
