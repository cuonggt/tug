import fs from 'node:fs'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

const devServer = 'http://localhost:5173'

// The Go server serves the build from public/build under /build/, and
// finds the dev server through public/hot.
export default defineConfig(({ command }) => ({
  base: command === 'build' ? '/build/' : '/',
  // public/ is the Go server's to serve; the build goes inside it.
  publicDir: false,
  plugins: [react(), tug()],
  build: {
    manifest: true,
    outDir: 'public/build',
    emptyOutDir: true,
    rolldownOptions: { input: 'resources/js/app.tsx' },
  },
  server: {
    port: 5173,
    strictPort: true,
    // The page comes from the Go server, so the files it loads must name
    // the dev server in full.
    origin: devServer,
  },
}))

// tug tells the Go server where the dev server is, by writing its URL to
// public/hot while it runs, and reloads the browser when tug dev has
// restarted the Go server, which it says by touching .tug/reload.
function tug(): Plugin {
  const hot = 'public/hot'
  const reload = '.tug/reload'
  return {
    name: 'tug',
    apply: 'serve',
    configureServer(server) {
      server.httpServer?.once('listening', () => fs.writeFileSync(hot, devServer))
      process.on('exit', () => fs.rmSync(hot, { force: true }))
      for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP'] as const) {
        process.on(signal, () => process.exit())
      }
      server.watcher.add(reload)
      const onReload = (file: string) => {
        if (file.endsWith(reload)) {
          server.ws.send({ type: 'full-reload' })
        }
      }
      server.watcher.on('add', onReload)
      server.watcher.on('change', onReload)
    },
  }
}
