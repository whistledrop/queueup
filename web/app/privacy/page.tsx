import Link from 'next/link'

// The privacy notice, in plain words.
//
// It says what is actually stored, checked against the code, rather than what a
// template assumes. When the product starts keeping something new, this page
// changes in the same commit.

export const metadata = {
  title: 'Privacy - QueueUp',
  description: 'What QueueUp keeps about you, why, and how to have it deleted.',
}

export default function PrivacyPage() {
  return (
    <div className="shell">
      <header className="top">
        <Link href="/" className="brand">Queue<span>Up</span></Link>
        <Link href="/" className="btn quiet">Back</Link>
      </header>

      <div className="card prose">
        <h1 style={{ marginTop: 0 }}>Privacy</h1>
        <p className="muted">Last updated 17 September 2026.</p>

        <p>
          QueueUp is a small, independent tool, currently a free beta. It is an
          unofficial third-party tool and has nothing to do with Facepunch
          Studios. This page says exactly what it keeps about you and why.
        </p>

        <h2>What we never have</h2>
        <ul>
          <li>
            <b>Your Steam password.</b> QueueUp never asks for it, never stores
            it, and has no way to use it.
          </li>
          <li>Payment details. Nothing is charged during the beta.</li>
          <li>
            Anything from inside the game. QueueUp does not read the game&apos;s
            memory or files, only the log file Rust writes as it runs, and that
            stays on your PC unless you send a problem report.
          </li>
        </ul>

        <h2>What we keep</h2>
        <ul>
          <li>
            <b>Your account:</b> your email address, and your password in a
            scrambled, one-way form that cannot be turned back into the password.
          </li>
          <li>
            <b>Your linked PC:</b> its Windows computer name, the QueueUp version
            it runs, its sleep setting, and when it was last connected.
          </li>
          <li>
            <b>Your joins:</b> which servers you asked to join, when, and each
            step of how it went. The same goes for saved servers and scheduled
            joins. This is what your phone shows you, and it is how we find out
            what went wrong when something does.
          </li>
          <li>
            <b>Feedback you send</b> from the feedback page.
          </li>
          <li>
            <b>Problem reports you choose to send</b> from the icon on your PC.
            A report contains QueueUp&apos;s own log and the most recent part of
            Rust&apos;s log. Rust writes your <b>Steam ID</b> into its log, and
            file locations in it can include your <b>Windows user name</b>, so
            a report can contain both. Nothing is sent unless you click it.
          </li>
        </ul>
        <p>
          The website sets one cookie, which keeps you signed in. There are no
          adverts, no tracking, and no analytics.
        </p>

        <h2>Why</h2>
        <p>
          Only to run QueueUp for you and to fix it when it goes wrong. Your
          email is used to sign you in. We do not sell your information or share
          it with anyone for marketing, and we will not email you marketing
          without asking first.
        </p>

        <h2>Where it lives</h2>
        <p>
          The service and its database run on Fly.io in London. The website is
          served by Netlify, and the PC download by GitHub. Server search asks
          Steam&apos;s public server list, which never receives anything about
          you. Your internet address is used for a few minutes to stop people
          hammering the sign-in page, and is not kept in the database.
        </p>

        <h2>How long</h2>
        <p>
          For as long as you have an account. Problem reports are deleted once
          they have been dealt with, and no later than the end of the beta.
        </p>

        <h2>Your rights</h2>
        <p>
          You can ask for a copy of what we hold about you, ask for it to be
          corrected, or ask for your account and everything in it to be
          deleted. Send the request from the{' '}
          <Link href="/feedback">feedback page</Link> while signed in, so we know
          it is really you. If you are unhappy with how your information is
          handled, you can complain to the UK Information Commissioner&apos;s
          Office at ico.org.uk.
        </p>

        <h2>Age</h2>
        <p>You need to be 13 or older to create an account.</p>
      </div>
    </div>
  )
}
