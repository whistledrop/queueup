// The front door, now that QueueUp is a phone app.
//
// One screen: what it is, the way to get it, and the picture. Most visitors
// arrive from a TikTok bio on a phone, so the store button for their phone is
// the page's whole job; the PC app, which the stores cannot install, gets its
// own clear link for the visit that comes from the PC.
//
// Until the app is in the stores, the store buttons cannot exist, so the page
// falls back to the email sign-up and keeps working for the traffic arriving
// today. Set these in Netlify and the buttons appear, no code change:
//   NEXT_PUBLIC_APP_STORE_URL    the App Store listing
//   NEXT_PUBLIC_PLAY_STORE_URL   the Google Play listing

import Link from 'next/link'
import HeroShot from './heroShot'
import StartForm from './startForm'
import StoreButtons from './storeButtons'
import { publishedTools } from '@/lib/tools'
import { SITE_DESCRIPTION, SITE_NAME, SITE_URL } from '@/lib/site'
import s from './appLanding.module.css'
// The email form's own styles live with the old landing page.
import l from './landing.module.css'

const APP_STORE = process.env.NEXT_PUBLIC_APP_STORE_URL ?? ''
const PLAY_STORE = process.env.NEXT_PUBLIC_PLAY_STORE_URL ?? ''

const STRUCTURED = {
  '@context': 'https://schema.org',
  '@type': 'SoftwareApplication',
  name: SITE_NAME,
  url: SITE_URL,
  description: SITE_DESCRIPTION,
  applicationCategory: 'GameApplication',
  operatingSystem: 'iOS, Android, Windows',
  offers: { '@type': 'Offer', price: '0', priceCurrency: 'GBP' },
}

export default function AppLanding() {
  const inStores = Boolean(APP_STORE || PLAY_STORE)
  return (
    <div className={s.page} data-landing="">
      <nav className={s.nav}>
        <span className="brand">
          Queue<span>Up</span>
        </span>
        <Link href="/login" className={s.signin}>Sign in</Link>
      </nav>

      <main className={s.hero}>
        <h1 className={s.title}>Join Rust servers from your phone.</h1>
        <p className={s.lede}>Your PC queues while you&apos;re out. You walk in and play.</p>

        {inStores ? (
          <StoreButtons appStore={APP_STORE} playStore={PLAY_STORE} />
        ) : (
          <>
            <p className={s.soon}>Coming soon to iPhone and Android.</p>
            <StartForm className={`${l.startForm} ${s.form}`} />
          </>
        )}
        <p className={s.free}>Free: 10 joins a month. Pro: 7 days free, then £4.99 a month.</p>

        <div className={s.shot}>
          <HeroShot />
        </div>

        {/* The visit from the PC: the one thing only this website can give. */}
        <Link href="/get" className={s.pc}>
          On your gaming PC? Get the QueueUp PC app <span aria-hidden="true">→</span>
        </Link>
      </main>

      <footer className={s.footer}>
        <p className={s.links}>
          {publishedTools().length > 0 && <><Link href="/tools">Free Rust tools</Link> · </>}
          <Link href="/guides">Rust guides</Link> · <Link href="/help">Help</Link> ·{' '}
          <Link href="/terms">Terms</Link> · <Link href="/privacy">Privacy</Link>
        </p>
        <p>
          QueueUp is an unofficial third party tool and is not affiliated with, endorsed by, or
          connected to Facepunch Studios. It never modifies or automates the game itself. Rust is a
          trademark of Facepunch Studios.
        </p>
      </footer>

      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(STRUCTURED).replace(/</g, '\\u003c') }}
      />
    </div>
  )
}
