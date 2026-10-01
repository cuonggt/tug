import { route } from '@/tug/routes'

// The user's own channel, /broadcasts in broadcasts.go, on one connection
// for the page, which every part of it that listens shares: the header's
// bell, a page's list. The browser connects again when the stream ends, as
// when the app restarts for a deploy, and every listener is called then
// too, as it may have missed an event: each reloads what it shows.

type Reload = () => void

const listeners = new Map<string, Set<Reload>>()
let events: EventSource | undefined

// Inertia's requests in flight, counted by the start and finish events it
// fires on the document for each, which a reload waits for: a reload as a
// form's visit loads the page it goes back to reads the session as that
// does, and both would show the flash the form left, two toasts of one
// message. waiting is what's to run as the last finishes, each once.
let visiting = 0
let counting = false
const waiting = new Set<Reload>()

function count() {
  if (counting) return
  counting = true
  document.addEventListener('inertia:start', () => visiting++)
  document.addEventListener('inertia:finish', () => {
    visiting = Math.max(visiting - 1, 0)
    if (visiting > 0) return
    const due = [...waiting]
    waiting.clear()
    due.forEach((r) => r())
  })
}

function reloadAll(all: Iterable<Reload>) {
  for (const r of all) {
    if (visiting > 0) waiting.add(r)
    else r()
  }
}

// listen calls reload as each event called name comes on the user's
// channel, and as the connection is made again, until the function it
// returns is called, once no request of Inertia's is in flight. The
// connection opens with the first listener, and closes with the last.
export function listen(name: string, reload: Reload): () => void {
  if (!events) {
    count()
    events = new EventSource(route('broadcasts'))
    let opened = false
    events.addEventListener('open', () => {
      if (opened) listeners.forEach((named) => reloadAll(named))
      opened = true
    })
  }
  let named = listeners.get(name)
  if (!named) {
    const all = new Set<Reload>()
    listeners.set(name, all)
    events.addEventListener(name, () => reloadAll(all))
    named = all
  }
  named.add(reload)
  return () => {
    named.delete(reload)
    waiting.delete(reload)
    if ([...listeners.values()].every((all) => all.size === 0)) {
      events?.close()
      events = undefined
      listeners.clear()
    }
  }
}
