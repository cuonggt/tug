import { useSyncExternalStore } from 'react'

// Appearance is how the app looks: light, dark, or as the system does. The
// choice is the browser's, kept in its localStorage, so it's there before
// the page paints, where app.html's script applies it.
export type Appearance = 'light' | 'dark' | 'system'

const key = 'appearance'
const listeners = new Set<() => void>()
const systemDark = () => window.matchMedia('(prefers-color-scheme: dark)')

function chosen(): Appearance {
  const saved = localStorage.getItem(key)
  return saved === 'light' || saved === 'dark' ? saved : 'system'
}

// apply puts the dark class on <html>, which Tailwind's dark: variant and
// the theme in app.css follow.
function apply(appearance: Appearance) {
  const dark = appearance === 'dark' || (appearance === 'system' && systemDark().matches)
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
}

function changed() {
  apply(chosen())
  listeners.forEach((listener) => listener())
}

// followAppearance keeps the page's appearance as the choice says while it
// runs: as the system's changes, for those who chose it, and as another
// tab changes the choice.
export function followAppearance() {
  systemDark().addEventListener('change', changed)
  window.addEventListener('storage', (event) => event.key === key && changed())
}

// useAppearance is the appearance chosen, and a way to choose another.
export function useAppearance() {
  const appearance = useSyncExternalStore(
    (listener) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    chosen,
    () => 'system' as const,
  )
  const setAppearance = (next: Appearance) => {
    if (next === 'system') {
      localStorage.removeItem(key)
    } else {
      localStorage.setItem(key, next)
    }
    changed()
  }
  return { appearance, setAppearance }
}
