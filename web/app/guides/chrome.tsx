import Link from 'next/link'
import { publishedTools } from '@/lib/tools'
import s from './guides.module.css'

// The header and footer every guide page shares. The button goes to the front
// page, where the free plan starts, rather than to sign-up directly: somebody
// arriving from a search has not seen what QueueUp is yet.
export function GuideNav() {
  return (
    <nav className={s.nav}>
      <Link href="/" className="brand">
        Queue<span>Up</span>
      </Link>
      <Link href="/" className={s.navCta}>Try it free</Link>
    </nav>
  )
}

export function GuideFooter() {
  return (
    <footer className={s.footer}>
      <p>
        {publishedTools().length > 0 && <><Link href="/tools">Free tools</Link> · </>}
        <Link href="/guides">All guides</Link> · <Link href="/">QueueUp</Link>
      </p>
      <p>
        QueueUp is an unofficial third party tool and is not affiliated with, endorsed by, or
        connected to Facepunch Studios. Rust is a trademark of Facepunch Studios.
      </p>
    </footer>
  )
}

/** The ask at the end of every guide: one thing, and free. */
export function GuideCta() {
  return (
    <div className={s.cta}>
      <h2>Queue for Rust from your phone</h2>
      <p>
        Tap join wherever you are and your gaming PC waits in the queue for you. Free for 10 joins a
        month, plus one free wipe join. It never touches the game.
      </p>
      <Link href="/" className={s.ctaBtn}>Try QueueUp free</Link>
    </div>
  )
}
