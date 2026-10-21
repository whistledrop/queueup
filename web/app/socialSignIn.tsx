'use client'

// "Continue with Google" and "Continue with Apple" on the website.
//
// The same buttons as the app, so somebody uses one way in everywhere and
// never ends up with two accounts. Each uses the provider's own official
// script and button; the provider hands back an ID token, the relay checks
// it, and the session lands in the same cookie as email sign-in.
//
// Each button appears only once its client id is configured:
//   NEXT_PUBLIC_GOOGLE_CLIENT_ID      the website's Google OAuth client id
//   NEXT_PUBLIC_APPLE_SERVICES_ID     the website's Apple Services ID
// With neither, this renders nothing and the email box carries on alone.

import { useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { claimCode } from '@/lib/promo'
import { track } from '@/lib/analytics'

const GOOGLE_ID = process.env.NEXT_PUBLIC_GOOGLE_CLIENT_ID ?? ''
const APPLE_ID = process.env.NEXT_PUBLIC_APPLE_SERVICES_ID ?? ''

type GoogleApi = {
  accounts: {
    id: {
      initialize: (o: { client_id: string; callback: (r: { credential: string }) => void; nonce?: string }) => void
      renderButton: (el: HTMLElement, o: Record<string, unknown>) => void
    }
  }
}
type AppleApi = {
  auth: {
    init: (o: Record<string, unknown>) => void
    signIn: () => Promise<{ authorization: { id_token: string } }>
  }
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    if (document.querySelector(`script[src="${src}"]`)) return resolve()
    const s = document.createElement('script')
    s.src = src
    s.async = true
    s.onload = () => resolve()
    s.onerror = () => reject(new Error('failed to load'))
    document.head.appendChild(s)
  })
}

function nonce(): string {
  const b = new Uint8Array(16)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

export const socialSignInAvailable = Boolean(GOOGLE_ID || APPLE_ID)

export default function SocialSignIn({ from = 'landing' }: { from?: string }) {
  const router = useRouter()
  const googleEl = useRef<HTMLDivElement>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function finish(provider: 'google' | 'apple', idToken: string, n: string) {
    setBusy(true)
    setError('')
    try {
      const res = await fetch('/api/auth/oauth', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ provider, id_token: idToken, nonce: n, code: claimCode() }),
      })
      const body = await res.json().catch(() => ({}))
      if (!res.ok) {
        setError(body.error ?? 'That sign-in didn’t work. Try again.')
        setBusy(false)
        return
      }
      if (body.created) {
        track('account_created', { from: `${from}_${provider}` })
        router.push('/plan')
        return
      }
      router.push('/')
      router.refresh()
    } catch {
      setError('We could not reach QueueUp. Check your connection.')
      setBusy(false)
    }
  }

  useEffect(() => {
    if (!GOOGLE_ID || !googleEl.current) return
    let live = true
    const n = nonce()
    loadScript('https://accounts.google.com/gsi/client')
      .then(() => {
        const g = (window as unknown as { google?: GoogleApi }).google
        if (!live || !g || !googleEl.current) return
        g.accounts.id.initialize({ client_id: GOOGLE_ID, nonce: n, callback: (r) => finish('google', r.credential, n) })
        g.accounts.id.renderButton(googleEl.current, {
          theme: 'filled_black', size: 'large', shape: 'pill', text: 'continue_with',
          width: Math.min(googleEl.current.offsetWidth || 320, 400),
        })
      })
      .catch(() => {})
    return () => {
      live = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function apple() {
    if (busy) return
    setError('')
    try {
      await loadScript('https://appleid.cdn-apple.com/appleauth/static/jsapi/appleid/1/en_US/appleid.auth.js')
      const a = (window as unknown as { AppleID?: AppleApi }).AppleID
      if (!a) throw new Error()
      const n = nonce()
      a.auth.init({
        clientId: APPLE_ID, scope: 'name email', usePopup: true, nonce: n,
        redirectURI: `${window.location.origin}/login`,
      })
      const r = await a.auth.signIn()
      await finish('apple', r.authorization.id_token, n)
    } catch (e) {
      // Closing Apple's window is a decision, not an error.
      if ((e as { error?: string })?.error === 'popup_closed_by_user') return
      setError('Apple sign-in didn’t work. Try again, or use your email.')
    }
  }

  if (!socialSignInAvailable) return null
  return (
    <div className="socialSignIn">
      <div className="socialOr"><span>or</span></div>
      {APPLE_ID && (
        <button type="button" className="appleBtn" onClick={apple} disabled={busy}>
          <svg width="16" height="16" viewBox="0 0 24 24" aria-hidden="true" fill="currentColor">
            <path d="M16.37 12.6c-.02-2.3 1.88-3.4 1.96-3.46-1.07-1.56-2.73-1.78-3.32-1.8-1.41-.14-2.76.83-3.47.83-.72 0-1.82-.81-3-.79-1.54.02-2.96.9-3.75 2.27-1.6 2.78-.41 6.9 1.15 9.15.76 1.1 1.67 2.34 2.86 2.3 1.15-.05 1.58-.74 2.97-.74 1.38 0 1.78.74 2.99.72 1.24-.02 2.02-1.12 2.77-2.23.87-1.28 1.23-2.52 1.25-2.58-.03-.01-2.39-.92-2.41-3.67zM14.1 5.86c.63-.77 1.06-1.83.94-2.89-.91.04-2.02.61-2.67 1.37-.58.67-1.09 1.76-.96 2.8 1.02.08 2.06-.52 2.69-1.28z" />
          </svg>
          Continue with Apple
        </button>
      )}
      {GOOGLE_ID && <div ref={googleEl} className="googleBtn" />}
      {error && <p className="startError">{error}</p>}
    </div>
  )
}
