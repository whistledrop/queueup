'use client'

// Bring your mates.
//
// The reason to build this before anybody has mates to bring is that the loop
// has to exist before the marketing does: a referral system added after the
// first wave of signups is a referral system the first wave never used.
//
// One mate who pays and puts a PC of their own on the end of it is one month
// at the lower price, up to three. Both halves matter and both are said here,
// because a reward whose conditions are hidden reads as a reward that did not
// arrive.

import { useEffect, useState } from 'react'
import { getReferral, type Referral } from '@/lib/api'
import { PLAN } from '@/lib/pricing'

export default function ReferAFriend() {
  const [ref, setRef] = useState<Referral | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    getReferral().then(setRef).catch(() => {})
  }, [])

  async function copy() {
    if (!ref?.link) return
    try {
      await navigator.clipboard.writeText(ref.link)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard blocked. The link is on screen to be copied by hand, which
      // is why it is shown rather than hidden behind the button.
    }
  }

  if (!ref || !ref.code) return null

  const intro = `${PLAN.symbol}${PLAN.intro.toFixed(2)}`
  const done = ref.earned >= ref.max

  return (
    <div className="card">
      <h2>Bring your mates</h2>
      <p className="muted" style={{ marginTop: 0 }}>
        They get their first month for {intro}. When one of them has paid and
        linked a PC of their own, your next month is {intro} too. Up to{' '}
        {ref.max} months.
      </p>

      <p className="getLink">{ref.code}</p>

      <button className="primary btn-wide" onClick={copy}>
        {copied ? 'Copied' : 'Copy your link'}
      </button>
      <p className="muted small" style={{ wordBreak: 'break-all', marginBottom: 0 }}>
        {ref.link}
      </p>

      <div className="server">
        <div className="row">
          <div>
            <div className="name">
              {done
                ? `All ${ref.max} earned`
                : `${ref.earned} of ${ref.max} earned`}
            </div>
            <div className="muted small">
              {ref.banked > 0
                ? `${ref.banked} ${ref.banked === 1 ? 'month' : 'months'} waiting, one at a time off your next bills.`
                : done
                  ? 'Thanks for bringing them.'
                  : 'Counted once your mate has paid and linked their PC.'}
            </div>
          </div>
          {ref.banked > 0 && <span className="pill good">{ref.banked}</span>}
        </div>
      </div>
    </div>
  )
}
