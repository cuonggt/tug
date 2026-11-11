package typegen

// liveTS is live.ts: the transport of Inertia's live props over the app's
// routes of events, the same for every app, which tug gen writes beside
// the rest, as it's tug's half of the protocol between them, the route's
// events and the page's socket ID, and changes with tug. Its types are
// its own, of the shape of Inertia's LiveTransport, which the app's
// createInertiaApp takes as they are, so it imports nothing.
const liveTS = header + `
// live is the transport of Inertia's live props, which createInertiaApp
// takes as its live: a live prop names a route of the app's by its path,
// as inertia.On does, and the events on it that refresh the prop, which
// the route sends with c.Events. One EventSource for each route, shared by
// every event the page's props listen for on it, is opened with the first
// and closed with the last. The browser connects again as one drops, and
// Inertia refreshes every live prop then, for the events it missed.
//
// Each page is a socket of its own, by an ID it makes, which Inertia sends
// with its requests as X-Socket-Id: an event published with
// inertia.ToOthers carries the ID of the page that made the change, and
// that page, which has the change already, drops it.

type Channel = { name: string; type: string; params?: Record<string, unknown> }
type Handler = (payload: unknown) => void

type Stream = {
  source: EventSource
  handlers: Map<string, Set<Handler>>
  listeners: Map<string, (message: MessageEvent) => void>
}

const streams = new Map<string, Stream>()
const watchers = new Set<(connected: boolean) => void>()

function tell(connected: boolean) {
  for (const watch of watchers) watch(connected)
}

function streamOf(url: string): Stream {
  let stream = streams.get(url)
  if (!stream) {
    const source = new EventSource(url)
    source.addEventListener('open', () => tell(true))
    source.addEventListener('error', () => tell(false))
    stream = { source, handlers: new Map(), listeners: new Map() }
    streams.set(url, stream)
  }
  return stream
}

function subscribe(channel: Channel, event: string, handler: Handler): () => void {
  const stream = streamOf(channel.name)
  let handlers = stream.handlers.get(event)
  if (!handlers) {
    const all = new Set<Handler>()
    const listener = (message: MessageEvent) => {
      let payload: unknown = null
      try {
        payload = JSON.parse(message.data)
      } catch {
        // an event with no JSON, a signal alone
      }
      for (const handle of all) handle(payload)
    }
    stream.source.addEventListener(event, listener)
    stream.handlers.set(event, all)
    stream.listeners.set(event, listener)
    handlers = all
  }
  handlers.add(handler)
  return () => {
    handlers.delete(handler)
    if (handlers.size > 0) return
    const listener = stream.listeners.get(event)
    if (listener) stream.source.removeEventListener(event, listener)
    stream.handlers.delete(event)
    stream.listeners.delete(event)
    if (stream.handlers.size === 0) {
      stream.source.close()
      streams.delete(channel.name)
    }
  }
}

let id: string | undefined

// socketId is the page's own ID, random, made as it's first asked for.
function socketId(): string {
  if (!id) {
    const bytes = new Uint8Array(12)
    crypto.getRandomValues(bytes)
    id = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
  }
  return id
}

function onStatusChange(watch: (connected: boolean) => void): () => void {
  watchers.add(watch)
  return () => {
    watchers.delete(watch)
  }
}

export const live = { subscribe, socketId, onStatusChange }
`
