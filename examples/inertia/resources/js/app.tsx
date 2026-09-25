import { createInertiaApp, type ResolvedComponent } from '@inertiajs/react'
import { createRoot } from 'react-dom/client'
import '../css/app.css'

// Each page is its own chunk, loaded when a visit needs it. The Go server's
// root template loads the first page's alongside this file.
const pages = import.meta.glob<{ default: ResolvedComponent }>('./pages/**/*.tsx')

createInertiaApp({
  title: (title) => (title ? `${title} · tug` : 'tug'),
  resolve: async (name) => {
    const load = pages[`./pages/${name}.tsx`]
    if (!load) {
      throw new Error(`There's no page component at resources/js/pages/${name}.tsx`)
    }
    return (await load()).default
  },
  setup({ el, App, props }) {
    createRoot(el).render(<App {...props} />)
  },
  progress: { color: '#0e7490' },
})
