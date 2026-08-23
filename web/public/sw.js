// QueueUp's service worker. Its one job: show push notifications when the tab
// is closed, and open the right page when one is tapped.

self.addEventListener('push', (event) => {
  let data = {}
  try {
    data = event.data ? event.data.json() : {}
  } catch {
    data = { title: 'QueueUp', body: event.data ? event.data.text() : '' }
  }
  event.waitUntil(
    self.registration.showNotification(data.title || 'QueueUp', {
      body: data.body || '',
      tag: data.tag || 'queueup',
      renotify: true,
      data: { url: data.url || '/' },
    }),
  )
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const url = (event.notification.data && event.notification.data.url) || '/'
  event.waitUntil(
    clients.matchAll({ type: 'window', includeUncontrolled: true }).then((tabs) => {
      for (const tab of tabs) {
        if ('focus' in tab) {
          tab.navigate(url)
          return tab.focus()
        }
      }
      return clients.openWindow(url)
    }),
  )
})
