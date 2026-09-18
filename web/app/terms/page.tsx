import Link from 'next/link'

// The terms, in plain words. Short on purpose: a Rust player should be able to
// read the whole thing in a minute and know exactly what they are paying for
// and how to stop.

export const metadata = {
  title: 'Terms - QueueUp',
  description: 'What you get, what it costs, and how to cancel.',
}

export default function TermsPage() {
  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn quiet">Back</Link>
      </header>

      <div className="card prose">
        <h1 style={{ marginTop: 0 }}>Terms</h1>
        <p className="muted">Last updated 18 September 2026.</p>

        <h2>What QueueUp is</h2>
        <p>
          A tool that starts Rust on your own Windows PC, pointed at the server
          you choose, and waits in the queue for you. It is an unofficial third
          party tool, not made or endorsed by Facepunch Studios.
        </p>

        <h2>What it costs</h2>
        <p>
          £1.99 for your first month, then £4.99 a month until you cancel. The
          £1.99 first month is once per account. Prices include any VAT due.
          Stripe takes the payment; we never see your card.
        </p>

        <h2>Cancelling</h2>
        <p>
          Cancel any time from <b>Manage subscription</b> on your dashboard. It
          stops at the end of the month you have paid for, and you keep access
          until then. Nothing more is taken.
        </p>
        <p>
          Changed your mind in the first 14 days after first subscribing? Email{' '}
          <a href="mailto:hello@queueuprust.com">hello@queueuprust.com</a> and we
          will refund you in full.
        </p>

        <h2>What we promise, and what we cannot</h2>
        <p>
          We work hard to get you in, and it usually does. But QueueUp depends on
          things we do not control: your PC staying on and awake, your internet,
          Steam, the game, and the server itself. A Rust update can change how
          the game behaves overnight. So we cannot promise that every single
          join succeeds, and we are not responsible for a wipe you miss. If it
          lets you down, tell us: that is how it gets fixed.
        </p>

        <h2>Fair use</h2>
        <p>
          One account, one PC. QueueUp never plays the game for you, reads its
          memory or changes its files, and you must not use it to do anything a
          server&apos;s own rules forbid. We can close an account that is used
          to abuse QueueUp or other people.
        </p>

        <h2>Your data</h2>
        <p>
          See the <Link href="/privacy">privacy page</Link>. The short version:
          your email, your PC&apos;s name, your joins, and nothing from inside
          the game. Never your Steam password.
        </p>

        <h2>Changes</h2>
        <p>
          If the price or these terms change, we will tell you before it affects
          you, and you can cancel before it does.
        </p>

        <h2>Contact</h2>
        <p>
          <a href="mailto:hello@queueuprust.com">hello@queueuprust.com</a>
        </p>
      </div>
    </div>
  )
}
