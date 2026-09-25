import fs from 'node:fs'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

const devServer = 'http://localhost:5173'

// The Go server serves the build from public/build under /build/, and
// finds the dev server through public/hot: see main.go.
export default defineConfig(({ command }) => ({
  base: command === 'build' ? '/build/' : '/',
  // public/ is the Go server's to serve; the build goes inside it.
  publicDir: false,
  plugins: [react(), hotFile('public/hot')],
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

// hotFile writes the dev server's URL to file while it runs, which tells the
// Go server to load the frontend from it rather than from the build.
function hotFile(file: string): Plugin {
  return {
    name: 'tug-hot-file',
    apply: 'serve',
    configureServer(server) {
      server.httpServer?.once('listening', () => fs.writeFileSync(file, devServer))
      process.on('exit', () => fs.rmSync(file, { force: true }))
      for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP'] as const) {
        process.on(signal, () => process.exit())
      }
    },
  }
}
