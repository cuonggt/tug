import createServer from '@inertiajs/react/server'
import { renderToString } from 'react-dom/server'
import { createApp } from './inertia'

// The app on the server: Inertia's SSR server, which renders the pages the
// Go server sends it to HTML, answering on 127.0.0.1 at the SSR_PORT the Go
// server starts it with. Under tug dev, Vite's dev server answers for it.
const render = await createApp()
if (typeof render !== 'function') {
  throw new Error('createInertiaApp returned no render function: ssr.tsx runs on the server, in Node')
}

// The pages come from the Go server, with the shared props tug gen typed.
createServer((page) => render(page as Parameters<typeof render>[0], renderToString), {
  host: '127.0.0.1',
  port: Number(process.env.SSR_PORT) || 13714,
})
