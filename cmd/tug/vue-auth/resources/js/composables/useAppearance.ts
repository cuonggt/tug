import { computed, onMounted, ref } from 'vue'

// Appearance is how the app looks: light, dark, or as the system does. The
// choice is the browser's, kept in its localStorage, so it's there before
// the page paints, where app.html's script applies it.
export type Appearance = 'light' | 'dark' | 'system'

const key = 'appearance'
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

// current is the choice as this page knows it, for the components that
// show it: read as the first of them is mounted, and again as it changes.
// It's only ever set in the browser, so the server's stays 'system'.
const current = ref<Appearance>('system')

function changed() {
  current.value = chosen()
  apply(current.value)
}

// followAppearance keeps the page's appearance as the choice says while it
// runs: as the system's changes, for those who chose it, and as another
// tab changes the choice.
export function followAppearance() {
  systemDark().addEventListener('change', changed)
  window.addEventListener('storage', (event) => event.key === key && changed())
}

// useAppearance is the appearance chosen, and a way to choose another.
// Until the component is mounted it's 'system', as on the server, which
// has no localStorage, so that a page rendered there hydrates as it came;
// then it's the browser's choice.
export function useAppearance() {
  const mounted = ref(false)
  onMounted(() => {
    current.value = chosen()
    mounted.value = true
  })
  const appearance = computed(() => (mounted.value ? current.value : 'system'))
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
