'use client'

// The headline word that changes: "Load into Rust servers from school /
// work / the gym / a restaurant / anywhere."
//
// The point is the list, not the motion. Each place is somewhere a person
// genuinely is when a server wipes, and "anywhere" is the one the sentence
// settles on, so it holds twice as long before going round again.
//
// The word takes exactly its own width, so the full stop sits right after it
// whichever word is showing. A slot fixed to the longest word ("a restaurant")
// left every shorter word floating in a gap. Anybody who has asked for less
// motion gets "anywhere", still, and the rotation never starts.

import { useEffect, useState } from 'react'
import s from './landing.module.css'

const WORDS = ['school', 'work', 'the gym', 'a restaurant', 'anywhere'] as const
const REST = WORDS.length - 1 // "anywhere", and where the still version stops
const HOLD = 2000
const HOLD_LAST = 4000

export default function Rotator() {
  const [i, setI] = useState(REST)
  const [moving, setMoving] = useState(false)

  useEffect(() => {
    if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return
    // Starts at the beginning of the list once it is allowed to move, rather
    // than jumping backwards from "anywhere" on the first tick.
    setI(0)
    setMoving(true)
  }, [])

  useEffect(() => {
    if (!moving) return
    const t = setTimeout(() => setI((n) => (n + 1) % WORDS.length), i === REST ? HOLD_LAST : HOLD)
    return () => clearTimeout(t)
  }, [i, moving])

  return (
    // The whole sentence is read out as one, with the word that happens to be
    // showing: a screen reader is not told about a word changing every two
    // seconds, which would be unusable.
    <em className={s.rotator}>
      <span key={i} className={s.rotatorWord}>
        {WORDS[i]}
      </span>
      <span className={s.rotatorStop}>.</span>
    </em>
  )
}
