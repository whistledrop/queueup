'use client'

// App Store and Google Play buttons. On a phone, the one for that phone comes
// first and the other steps back; elsewhere both sit side by side. Decided in
// the browser, because the server cannot see which phone is asking without
// guessing from headers that lie.
//
// Plain buttons for now. Apple's and Google's own badge artwork, downloaded
// from their marketing sites, should replace these before launch: both ask
// that their badges be used as supplied.

import { useEffect, useState } from 'react'
import { track } from '@/lib/analytics'
import s from './appLanding.module.css'

export default function StoreButtons({ appStore, playStore }: { appStore: string; playStore: string }) {
  const [os, setOs] = useState<'ios' | 'android' | 'other'>('other')
  useEffect(() => {
    const ua = navigator.userAgent
    setOs(/iPhone|iPad|iPod/i.test(ua) ? 'ios' : /Android/i.test(ua) ? 'android' : 'other')
  }, [])

  const apple = appStore && (
    <a
      key="ios"
      href={appStore}
      className={`${s.store} ${os === 'android' ? s.storeQuiet : ''}`}
      onClick={() => track('store_clicked', { store: 'app_store', device: os })}
    >
      <small>Download on the</small>App Store
    </a>
  )
  const google = playStore && (
    <a
      key="android"
      href={playStore}
      className={`${s.store} ${os === 'ios' ? s.storeQuiet : ''}`}
      onClick={() => track('store_clicked', { store: 'google_play', device: os })}
    >
      <small>Get it on</small>Google Play
    </a>
  )
  return <div className={s.stores}>{os === 'android' ? [google, apple] : [apple, google]}</div>
}
