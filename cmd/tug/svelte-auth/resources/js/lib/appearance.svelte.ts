// Appearance is how the app looks: light, dark, or as the system does. The
// choice is the browser's, kept in its localStorage, so it's there before
// the page paints, where app.html's script applies it.
export type Appearance = 'light' | 'dark' | 'system'

const key = 'appearance'
const systemDark = () => window.matchMedia('(prefers-color-scheme: dark)')

// current is the choice as the components show it. It's 'system' until
// followAppearance reads the browser's, as it is on the server, which can't
// know it: a page the server rendered is then the same as the browser's
// first render of it, which takes it over.
let current = $state<Appearance>('system')

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
  current = chosen()
  apply(current)
}

// followAppearance has the components show the choice, and keeps the
// page's appearance as the choice says while it runs: as the system's
// changes, for those who chose it, and as another tab changes the choice.
// app.ts calls it once the app is mounted.
export function followAppearance() {
  current = chosen()
  systemDark().addEventListener('change', changed)
  window.addEventListener('storage', (event) => event.key === key && changed())
}

// appearance is the appearance chosen, and a way to choose another.
export const appearance = {
  get current(): Appearance {
    return current
  },
  set(next: Appearance) {
    if (next === 'system') {
      localStorage.removeItem(key)
    } else {
      localStorage.setItem(key, next)
    }
    changed()
  },
}
