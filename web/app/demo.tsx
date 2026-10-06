// The real thing, recorded: a phone on the left, the PC on the right, one tap
// and the game loading itself.
//
// Nothing here until the file exists. Drop the recording in as
// web/public/demo.mp4 with a still frame as web/public/demo.jpg, set SRC
// below, and the section appears. Until then the page simply does not have
// one, which is better than a box that says a video is coming.
//
// Keep it short and quiet: muted, looping, no sound to turn on, and the still
// showing before a single byte of video is fetched, so a phone on a train
// loads the page just as fast as it does today.

import s from './landing.module.css'

/** Set to '/demo.mp4' once the file is in web/public. */
const SRC = ''
const POSTER = '/demo.jpg'

export default function Demo() {
  if (!SRC) return null
  return (
    <section className={s.section}>
      <p className={s.kicker}>See it</p>
      <h2>One tap, and the PC does the rest</h2>
      <div className={s.demoWrap}>
        <video
          className={s.demo}
          src={SRC}
          poster={POSTER}
          autoPlay
          muted
          loop
          playsInline
          // Nothing is fetched until it is wanted; the still carries the page.
          preload="none"
          aria-label="A phone tapping join, and a PC loading into a Rust server"
        />
      </div>
    </section>
  )
}
