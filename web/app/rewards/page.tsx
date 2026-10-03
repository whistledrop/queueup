'use client'

// Rewards.
//
// One reward today and more later, so the screen is a list of deals rather
// than a page about referrals. The next one is an entry in this list.
//
// The months are drawn as slots instead of written as a sentence, because
// "how many have I got left" means two different things to two different
// people: how many are still coming off my bills, and how many more could I
// still earn. Three slots in three states answer both without choosing.

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import Nav, { Footer } from '../nav'
import Gift from '../gift'
import { getReferral, type Referral } from '@/lib/api'
import { PLAN } from '@/lib/pricing'
import s from './rewards.module.css'

export default function RewardsPage() {
  const [ref, setRef] = useState<Referral | null>(null)
  const [copied, setCopied] = useState(false)

  const load = useCallback(() => {
    getReferral().then(setRef).catch(() => {})
  }, [])
  useEffect(load, [load])

  async function copy() {
    if (!ref?.link) return
    try {
      await navigator.clipboard.writeText(ref.link)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard refused. The link is on screen in a field they can select,
      // which is why it is shown rather than hidden behind the button.
    }
  }

  const intro = `${PLAN.symbol}${PLAN.intro.toFixed(2)}`
  const slots = Array.from({ length: ref?.max ?? 3 }, (_, i) => {
    if (!ref) return 'locked'
    if (i < ref.used) return 'used'
    if (i < ref.earned) return 'waiting'
    return 'locked'
  })

  return (
    <div className="shell">
      <Nav />

      <div className="card">
        <h2>Rewards</h2>
        <p className={s.intro}>
          Things you can do to pay less for QueueUp. More coming.
        </p>

        <div className={s.reward}>
          <div className={s.head}>
            <span className={s.badge}>
              <Gift size={21} />
            </span>
            <div style={{ minWidth: 0 }}>
              <h3 className={s.title}>Bring your mates</h3>
              <p className={s.what}>
                They get their first month for {intro}. When one of them has
                paid and linked a PC of their own, one of your months drops to{' '}
                {intro} too.
              </p>
            </div>
          </div>

          <div className={s.slots}>
            {slots.map((state, i) => (
              <div
                key={i}
                className={`${s.slot} ${state === 'used' ? s.used : ''} ${
                  state === 'waiting' ? s.waiting : ''
                }`}
              >
                <b>{intro}</b>
                {state === 'used' ? 'Used' : state === 'waiting' ? 'Waiting' : 'Locked'}
              </div>
            ))}
          </div>

          {ref?.link ? (
            <>
              <div className={s.linkRow}>
                <input
                  readOnly
                  value={ref.link}
                  onFocus={(e) => e.currentTarget.select()}
                  aria-label="Your invite link"
                />
                <button className="primary" onClick={copy}>
                  {copied ? 'Copied' : 'Copy'}
                </button>
              </div>
              <p className={s.terms}>
                {ref.banked > 0
                  ? `${ref.banked} ${ref.banked === 1 ? 'month is' : 'months are'} waiting. They come off your next bills, one at a time.`
                  : ref.remaining > 0
                    ? `${ref.remaining} more to earn. A mate counts once they have paid and linked their PC.`
                    : 'All earned. Thanks for bringing them.'}
              </p>
            </>
          ) : (
            <p className={s.terms}>
              Your invite link appears here once payments are switched on.
            </p>
          )}
        </div>

        <div className={s.soon}>
          More rewards are on the way. If there is something you would want to
          earn, say so on the <Link href="/feedback">feedback page</Link>.
        </div>
      </div>

      <Footer />
    </div>
  )
}
