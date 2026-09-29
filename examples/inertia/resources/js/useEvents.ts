import { useEffect, useEffectEvent } from 'react'

// useEvents follows the server-sent events at url, a route of Go's
// c.Events, while the page is shown, and calls reload as each event named
// in names comes: an event says what changed, and reload asks the server
// for what the page shows of it. The browser connects again when the
// stream ends, as when the server restarts, and reload is called then too,
// for what the page missed meanwhile.
export function useEvents(url: string, names: string[], reload: () => void) {
  const changed = useEffectEvent(reload)
  const listened = names.join(' ')
  useEffect(() => {
    const events = new EventSource(url)
    let opened = false
    events.addEventListener('open', () => {
      if (opened) changed()
      opened = true
    })
    for (const name of listened.split(' ')) {
      events.addEventListener(name, () => changed())
    }
    return () => events.close()
  }, [url, listened])
}
