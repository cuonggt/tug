import { route } from '@/tug/routes'

// The user's own channel, /broadcasts in broadcasts.go, on one connection
// for the page, which every part of it that listens shares: the header's
// bell, a page's list. The browser connects again when the stream ends, as
// when the app restarts for a deploy, and every listener is called then
// too, as it may have missed an event: each reloads what it shows.

type Reload = () => void

const listeners = new Map<string, Set<Reload>>()
let events: EventSource | undefined

// listen calls reload as each event called name comes on the user's
// channel, and as the connection is made again, until the function it
// returns is called. The connection opens with the first listener, and
// closes with the last.
export function listen(name: string, reload: Reload): () => void {
  if (!events) {
    events = new EventSource(route('broadcasts'))
    let opened = false
    events.addEventListener('open', () => {
      if (opened) listeners.forEach((named) => named.forEach((r) => r()))
      opened = true
    })
  }
  let named = listeners.get(name)
  if (!named) {
    const all = new Set<Reload>()
    listeners.set(name, all)
    events.addEventListener(name, () => all.forEach((r) => r()))
    named = all
  }
  named.add(reload)
  return () => {
    named.delete(reload)
    if ([...listeners.values()].every((all) => all.size === 0)) {
      events?.close()
      events = undefined
      listeners.clear()
    }
  }
}
