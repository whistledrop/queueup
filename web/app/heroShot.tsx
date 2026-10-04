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
                    {p.pc === 'world' && <Spawn />}
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

// Where a Rust player wakes up: a beach, at dawn, with a rock.
//
// Drawn rather than screenshotted. The game's art belongs to Facepunch, and
// QueueUp's whole standing with them rests on being the tool that touches
// nothing of theirs. It does not need to be exact; it needs to be recognisable
// at the size of a thumbnail, which is the opposite problem to accuracy.
//
// The first version was accurate and unreadable: a dark sky over a dark sea
// over dark sand is a black rectangle from two feet away. This one puts the
// sun on the horizon and the light on the sand, because what a beach at dawn
// actually looks like from across a room is bright.
function Spawn() {
  return (
    <svg className={s.spawn} viewBox="0 0 160 90" preserveAspectRatio="xMidYMid slice" aria-hidden="true">
      <defs>
        <linearGradient id="qsky" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#2f4a6b" />
          <stop offset="42%" stopColor="#8f7f8e" />
          <stop offset="78%" stopColor="#e8a06a" />
          <stop offset="100%" stopColor="#f6c98d" />
        </linearGradient>
        <linearGradient id="qsea" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#c99a72" />
          <stop offset="55%" stopColor="#4a6f83" />
          <stop offset="100%" stopColor="#355a6e" />
        </linearGradient>
        <linearGradient id="qsand" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="#c6a878" />
          <stop offset="100%" stopColor="#7d6648" />
        </linearGradient>
      </defs>

      <rect width="160" height="90" fill="url(#qsky)" />
      <circle cx="104" cy="49" r="9" fill="#fff1cf" />
      <circle cx="104" cy="49" r="16" fill="#ffd9a0" opacity="0.3" />

      {/* A headland, so the sea has something to end against. */}
      <path d="M0 50q16-7 30-4t22 5H0z" fill="#3c4f5e" opacity="0.85" />
      <path d="M8 47l3-5 3 5zM16 48l3-6 4 6zM25 48l3-5 3 5z" fill="#2e3f4c" />

      <rect y="50" width="160" height="14" fill="url(#qsea)" />
      <path d="M92 55h24M62 58h20M112 60h34M30 61h26" stroke="#ffe6bf" strokeWidth="0.9" opacity="0.55" />

      {/* Wet sand catching the light, then the dry beach. */}
      <path d="M0 64q42-4 82-1t78 2v25H0z" fill="url(#qsand)" />
      <path d="M0 64q42-4 82-1t78 2v3q-38-4-78-2T0 67z" fill="#e8cf9e" opacity="0.5" />

      {/* The rock you wake up next to. */}
      <path d="M22 80l6-13 9-3 10 6 4 10z" fill="#6b655c" />
      <path d="M28 67l9-3 5 4-8 5z" fill="#847d72" />
      <path d="M22 80l6-13 4 2-3 11z" fill="#57524a" />

      {/* And you, holding the rock you woke up with. */}
      <g fill="#33281f">
        <circle cx="62" cy="68" r="2.4" />
        <path d="M60.4 70.7h3.2l1.1 8h-5.4z" />
        <path d="M60.2 78.7h1.7l-.5 5.6h-1.8zM62.7 78.7h1.7l.6 5.6h-1.8z" />
        <path d="M64.3 71.6l3.9 1.8-.7 1.5-3.8-1.7z" />
      </g>
      <ellipse cx="62" cy="84.6" rx="4.6" ry="1" fill="#000" opacity="0.18" />
      <ellipse cx="34" cy="81" rx="12" ry="1.6" fill="#000" opacity="0.14" />
    </svg>
  )
}
