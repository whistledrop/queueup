'use client'

// Turning notifications on: register the service worker, ask the browser's
// push service for a subscription, hand it to the relay. All of it is the
// user's explicit choice; nothing here runs until they tap the button.

import { api } from './api'

export type PushState = 'unsupported' | 'relay-off' | 'off' | 'on' | 'denied'

export async function pushState(): Promise<PushState> {
  if (!('serviceWorker' in navigator) || !('PushManager' in window)) return 'unsupported'
  const cfg = await api<{ enabled: boolean }>('/api/push/config').catch(() => null)
  if (!cfg?.enabled) return 'relay-off'
  if (Notification.permission === 'denied') return 'denied'
  const reg = await navigator.serviceWorker.getRegistration()
  const sub = await reg?.pushManager.getSubscription()
  return sub ? 'on' : 'off'
}

export async function enablePush(): Promise<void> {
  const cfg = await api<{ enabled: boolean; public_key: string }>('/api/push/config')
  if (!cfg.enabled) throw new Error("Notifications aren't set up on this relay yet.")

  const reg = await navigator.serviceWorker.register('/sw.js')
  await navigator.serviceWorker.ready

  const permission = await Notification.requestPermission()
  if (permission !== 'granted') {
    throw new Error('Notifications are blocked for this site in your browser settings.')
  }

  const sub = await reg.pushManager.subscribe({
    userVisibleOnly: true,
    applicationServerKey: urlBase64ToUint8Array(cfg.public_key),
  })
  await api('/api/push/subscribe', { method: 'POST', body: JSON.stringify(sub.toJSON()) })
}

export async function disablePush(): Promise<void> {
  const reg = await navigator.serviceWorker.getRegistration()
  const sub = await reg?.pushManager.getSubscription()
  if (sub) {
    await api('/api/push/unsubscribe', {
      method: 'POST',
      body: JSON.stringify({ endpoint: sub.endpoint }),
    }).catch(() => {})
    await sub.unsubscribe()
  }
}

export async function sendTestPush(): Promise<void> {
  await api('/api/push/test', { method: 'POST' })
}

function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padding = '='.repeat((4 - (base64.length % 4)) % 4)
  const raw = atob((base64 + padding).replace(/-/g, '+').replace(/_/g, '/'))
  const out = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i)
  return out
}
