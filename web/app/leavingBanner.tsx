'use client'

// The reminder that an account is counting down to deletion.
//
// It rides with the header, so it is on every screen rather than only the one
// where they pressed the button. The whole value of the week is that somebody
// who asked on a bad night gets asked again on a better one, and that only
// works if they cannot miss it.

import { useCallback, useEffect, useState } from 'react'
import { api } from '@/lib/api'

export default function LeavingBanner() {
  const [when, setWhen] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const check = useCallback(async () => {
    try {
      const me = await api<{ erase_after?: string }>('/api/auth/me')
      setWhen(me.erase_after ?? null)
    } catch {
      // Signed out, or the relay is unreachable. Either way there is nothing
      // useful to say here, and this must never be what breaks a page.
    }
  }, [])

  useEffect(() => {
    check()
  }, [check])

  if (!when) return null

  const day = new Date(when).toLocaleDateString(undefined, {
    weekday: 'long',
    day: 'numeric',
    month: 'long',
  })

  async function keep() {
    setBusy(true)
    try {
      await api('/api/account/erase/cancel', { method: 'POST' })
      setWhen(null)
    } catch {
      setBusy(false)
    }
  }

  return (
    <div className="leaving">
      <div>
        <b>Your account will be deleted on {day}.</b> Everything goes then: your
        PC, your joins, your schedules and your saved servers. Until then
        nothing has changed and QueueUp works as normal.
      </div>
      <button onClick={keep} disabled={busy}>
        {busy ? 'Keeping' : 'Keep my account'}
      </button>
    </div>
  )
}
