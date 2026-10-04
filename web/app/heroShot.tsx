'use client'

// The hero picture, playing.
//
// A still of a phone next to a monitor says the two things exist. It does not
// say that pressing one makes the other work, which is the whole product. So
// it runs, and what runs on the monitor is a PC: a desktop with a taskbar,
// Rust opening in a window, going fullscreen, loading, queueing, and landing
// in the world. The phone says the same thing at the same moment.
//
// The game's own look is deliberately not copied: no Facepunch logo, menu art
// or interface. A window with its name on it and a loading bar is a true
// picture of what happens on the PC and is ours to draw.

import { useEffect, useState } from 'react'
import s from './landing.module.css'

type Phase = {
  ms: number
  tap?: boolean
  phone: { state: string; sub?: string; done?: boolean; join?: boolean }
  // What the monitor is showing.
  pc: 'desktop' | 'opening' | 'loading' | 'queue' | 'world'
  progress?: number
  queue?: number
}

const SCRIPT: Phase[] = [
  { ms: 1500, phone: { state: '198 / 200', join: true }, pc: 'desktop' },
  { ms: 650, tap: true, phone: { state: '198 / 200', join: true }, pc: 'desktop' },
  { ms: 1500, phone: { state: 'Launching Rust' }, pc: 'opening', progress: 42 },
  { ms: 1400, phone: { state: 'Connecting' }, pc: 'loading', progress: 78 },
  { ms: 1500, phone: { state: 'In the queue', sub: '212 ahead' }, pc: 'queue', queue: 212 },
  { ms: 1200, phone: { state: 'In the queue', sub: '9 ahead' }, pc: 'queue', queue: 9 },
  { ms: 1300, phone: { state: 'Loading the world' }, pc: 'loading', progress: 96 },
  { ms: 2800, phone: { state: "You're in", sub: 'Slot is being held', done: true }, pc: 'world' },
]

// The frame shown to anybody who has asked for less movement, and a sensible
// thing to be caught on: the queue, which explains the product standing still.
const RESTING = 4

export default function HeroShot() {
  const [i, setI] = useState(0)
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
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

  const p = SCRIPT[i]
  const fullscreen = p.pc !== 'desktop' && p.pc !== 'opening'

  return (
    <div className={s.shot}>
      <div className={s.shotSide}>
        <div className={s.iphone}>
          <div className={s.island} />
          <div className={s.iphoneScreen}>
            <div className={s.shotServer}>Rustopia EU Main</div>
            {p.phone.join ? (
              <>
                <div className={s.shotPop}>{p.phone.state}</div>
                <div className={`${s.shotJoin} ${p.tap ? s.shotJoinTap : ''}`}>
                  Join
                  {p.tap && <span className={s.tapRipple} />}
                </div>
              </>
            ) : (
              <>
                <div className={`${s.shotState} ${p.phone.done ? s.shotStateDone : ''}`}>
                  {p.phone.state}
                </div>
                {p.phone.sub && <div className={s.shotPop}>{p.phone.sub}</div>}
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
              <div className={s.winDesktop}>
                <div className={s.winIcon} />
                <div className={s.winIcon} />

                {p.pc === 'opening' && (
                  <div className={s.winApp}>
                    <div className={s.winAppBar}>Rust</div>
                    <div className={s.winAppBody}>
                      <div className={s.bar}>
                        <span style={{ width: `${p.progress ?? 0}%` }} />
                      </div>
                      <div className={s.winAppNote}>Starting</div>
                    </div>
                  </div>
                )}

                {fullscreen && (
                  <div className={`${s.gameFull} ${p.pc === 'world' ? s.gameWorld : ''}`}>
                    {p.pc === 'loading' && (
                      <>
                        <div className={s.gameText}>Loading the world</div>
                        <div className={s.bar}>
                          <span style={{ width: `${p.progress ?? 0}%` }} />
                        </div>
                      </>
                    )}
                    {p.pc === 'queue' && (
                      <>
                        <div className={s.gameText}>Waiting in queue</div>
                        <div className={s.gameBig}>{p.queue}</div>
                        <div className={s.gameDim}>players ahead of you</div>
                      </>
                    )}
                    {p.pc === 'world' && <div className={s.gameHorizon} />}
                  </div>
                )}

                <div className={s.winTaskbar}>
                  <span className={s.winStart} />
                  <span className={`${s.winTask} ${p.pc !== 'desktop' ? s.winTaskOn : ''}`} />
                  <span className={s.winClock}>19:04</span>
                </div>
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
