'use client'

// Settings.
//
// Before this page existed the account controls were wherever they had first
// been needed: unlinking a PC was a small red word at the bottom of the home
// screen, managing the subscription was the button under it, and the email
// address was grey six-point text above the footer. Two things people expect
// to be able to do to their own account were not possible at all: change the
// password without going through "I forgot it", and leave.
//
// So this is the one place for everything about the account rather than about
// playing, and the home screen goes back to being about joining servers.

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import Nav, { Footer } from '../nav'
import { api, getBilling, openManageSubscription, type Billing, type Device } from '@/lib/api'

export default function SettingsPage() {
  const router = useRouter()
  const [email, setEmail] = useState('')
  const [devices, setDevices] = useState<Device[]>([])
  const [billing, setBilling] = useState<Billing | null>(null)
  const [error, setError] = useState('')
  const [done, setDone] = useState('')

  const load = useCallback(async () => {
    try {
      const [me, d] = await Promise.all([
        api<{ email: string }>('/api/auth/me'),
        api<{ devices: Device[] }>('/api/devices'),
      ])
      setEmail(me.email)
      setDevices(d.devices ?? [])
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  useEffect(() => {
    load()
    getBilling().then(setBilling).catch(() => {})
  }, [load])

  async function signOut() {
    await fetch('/api/auth/logout', { method: 'POST' })
    router.push('/login')
    router.refresh()
  }

  // Unlinking is for a new PC, a sold PC, or starting again. The confirm says
  // exactly what stops, so nobody loses a wipe join by surprise.
  async function unlinkPC(id: string, name: string) {
    const ok = confirm(
      `Unlink ${name}?\n\n` +
        'QueueUp on that PC stops working and shows a new code. Any join running ' +
        'on it now is stopped, and joins scheduled for it are cancelled.\n\n' +
        'You can link this PC, or a different one, again at any time.',
    )
    if (!ok) return
    try {
      await api(`/api/devices/${id}/revoke`, { method: 'POST' })
      setDone(`${name} is unlinked.`)
      await load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <div className="shell">
      <Nav />

      {error && <div className="error">{error}</div>}
      {done && <div className="notice">{done}</div>}

      <div className="card">
        <h2>Account</h2>
        <div className="row">
          <div style={{ minWidth: 0 }}>
            <div className="name">{email || 'Loading'}</div>
            <div className="muted small">
              Joins, schedules and receipts all go to this address.
            </div>
          </div>
          <button onClick={signOut} style={{ minHeight: 40, padding: '6px 14px' }}>
            Sign out
          </button>
        </div>
      </div>

      <ChangePassword onDone={setDone} onError={setError} />

      <div className="card">
        <h2>Your PC</h2>
        {devices.length === 0 && (
          <p className="muted" style={{ margin: 0 }}>
            No PC is linked. <Link href="/">Link one from the home screen.</Link>
          </p>
        )}
        {devices.map((d) => (
          <div className="server" key={d.id}>
            <div className="row">
              <div style={{ minWidth: 0 }}>
                <div className="name">
                  <span className={`dot ${d.online ? 'on' : 'off'}`} />
                  {d.name}
                </div>
                <div className="muted small">
                  {d.online ? 'Online and ready' : 'Offline'} · QueueUp {d.agent_version}
                </div>
              </div>
              <button
                onClick={() => unlinkPC(d.id, d.name)}
                style={{ minHeight: 40, padding: '6px 14px', color: 'var(--bad)' }}
              >
                Unlink
              </button>
            </div>
          </div>
        ))}
      </div>

      <div className="card">
        <h2>Subscription</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          {billing === null
            ? 'Loading'
            : !billing.enabled
              ? 'QueueUp is a free beta. Nobody is being charged.'
              : billing.subscribed
                ? `Subscribed. ${billing.price_line}`
                : 'Not subscribed, so joining is locked.'}
        </p>
        {billing?.can_manage && (
          <button
            className="btn-wide"
            onClick={() => openManageSubscription().catch((e) => setError((e as Error).message))}
          >
            Manage subscription
          </button>
        )}
        {billing?.enabled && !billing.subscribed && (
          <Link href="/subscribe" className="btn btn-primary btn-wide">
            Subscribe
          </Link>
        )}
      </div>

      <DeleteAccount canManage={!!billing?.can_manage} onError={setError} />

      <Footer />
    </div>
  )
}

/* Changing a password while signed in. It takes the current one on purpose:
   a session left open on a shared PC should not be enough to lock the real
   owner out of their own account. */
function ChangePassword({
  onDone,
  onError,
}: {
  onDone: (s: string) => void
  onError: (s: string) => void
}) {
  const [open, setOpen] = useState(false)
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    onError('')
    try {
      const res = await fetch('/api/auth/password', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ current, next }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) throw new Error(body.error ?? 'That did not work.')
      onDone(body.status)
      setOpen(false)
      setCurrent('')
      setNext('')
    } catch (err) {
      onError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card">
      <h2>Password</h2>
      {!open ? (
        <button className="btn-wide" onClick={() => setOpen(true)}>
          Change password
        </button>
      ) : (
        <form onSubmit={submit} className="stack">
          <input
            type="password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            placeholder="Current password"
            autoComplete="current-password"
            aria-label="Current password"
          />
          <input
            type="password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            placeholder="New password"
            autoComplete="new-password"
            aria-label="New password"
          />
          <button type="submit" className="primary btn-wide" disabled={busy || !current || !next}>
            {busy ? 'Changing' : 'Change password'}
          </button>
          <button type="button" className="quiet" onClick={() => setOpen(false)}>
            Cancel
          </button>
          <p className="muted small" style={{ marginBottom: 0 }}>
            Anything else signed in to your account gets signed out. You stay
            signed in here.
          </p>
        </form>
      )}
      <p className="muted small" style={{ marginBottom: 0 }}>
        Forgotten it? <Link href="/forgot">Get a reset link by email.</Link>
      </p>
    </div>
  )
}

/* Leaving. Kept behind its own two gates and put last, because it is the one
   thing on this page that cannot be undone. */
function DeleteAccount({
  canManage,
  onError,
}: {
  canManage: boolean
  onError: (s: string) => void
}) {
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    onError('')
    try {
      await api('/api/account/erase', {
        method: 'POST',
        body: JSON.stringify({ password, confirm }),
      })
      // The session died with the account; drop the cookie too.
      await fetch('/api/auth/logout', { method: 'POST' })
      router.push('/login?deleted=1')
      router.refresh()
    } catch (err) {
      onError((err as Error).message)
      setBusy(false)
    }
  }

  return (
    <div className="card">
      <h2>Delete account</h2>
      {!open ? (
        <>
          <p className="muted" style={{ marginTop: 0 }}>
            Removes your account and everything we hold about it: your linked
            PC, every join and its timeline, your schedules, your saved servers
            and anything you have sent us. It cannot be undone.
          </p>
          <button
            className="btn-wide"
            style={{ color: 'var(--bad)' }}
            onClick={() => setOpen(true)}
          >
            Delete my account
          </button>
        </>
      ) : (
        <form onSubmit={submit} className="stack">
          {canManage && (
            <p className="muted small" style={{ marginTop: 0 }}>
              Cancel your subscription first, under Subscription above.
              Otherwise you would keep being charged with no account left to
              cancel from.
            </p>
          )}
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Your password"
            autoComplete="current-password"
            aria-label="Your password"
          />
          <input
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            placeholder="Type DELETE"
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
            aria-label="Type DELETE to confirm"
          />
          <button
            type="submit"
            className="btn-wide"
            style={{ background: 'var(--bad)', color: '#fff' }}
            disabled={busy || !password || confirm.trim().toUpperCase() !== 'DELETE'}
          >
            {busy ? 'Deleting' : 'Delete my account for good'}
          </button>
          <button type="button" className="quiet" onClick={() => setOpen(false)}>
            Keep my account
          </button>
        </form>
      )}
    </div>
  )
}
