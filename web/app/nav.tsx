'use client'

// The one header every signed-in page shares. Before this, each page had its
// own ad-hoc top bar (a Back button here, a Sign out there) and moving around
// the app meant learning each page's idea of navigation. One bar, three
// destinations, always in the same place.

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { BETA } from '@/lib/pricing'
import HelpBot from './helpBot'

const tabs = [
  { href: '/', label: 'Home' },
  { href: '/servers', label: 'Servers' },
  { href: '/schedule', label: 'Schedule' },
]

export default function Nav() {
  const router = useRouter()
  const pathname = usePathname()

  async function signOut() {
    await fetch('/api/auth/logout', { method: 'POST' })
    router.push('/login')
    router.refresh()
  }

  return (
    <header className="top">
      <Link href="/" className="brand">
        Queue<span>Up</span>
        {BETA && <span className="beta">beta</span>}
      </Link>
      <nav className="tabs" aria-label="Main">
        {tabs.map((t) => (
          <Link
            key={t.href}
            href={t.href}
            className={`tab ${pathname === t.href ? 'active' : ''}`}
          >
            {t.label}
          </Link>
        ))}
      </nav>
      <button className="quiet" onClick={signOut}>
        Sign out
      </button>

    </header>
  )
}

/** The one line every page carries at the bottom. */
export function Footer() {
  // The help bubble rides with the footer rather than the top bar, because
  // /help and /feedback have no top bar and they are precisely where somebody
  // who is stuck ends up. The one page it would be silly on is the one that
  // is already the assistant.
  const pathname = usePathname()
  return (
    <footer className="foot">
      {pathname !== '/ask' && <HelpBot />}
      <Link href="/ask">Ask a question</Link>
      {' · '}
      <Link href="/help">Help</Link>
      {' · '}
      <Link href="/feedback">Send feedback</Link>
      {' · '}
      <Link href="/privacy">Privacy</Link>
      {' · '}
      <Link href="/terms">Terms</Link>
      <br />
      QueueUp is an unofficial third-party tool, not affiliated with Facepunch
      Studios. It never modifies or automates the game itself.
    </footer>
  )
}
