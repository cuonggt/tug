import path from 'node:path'

// The apps the suite drives: the auth starter, rendered on the server too,
// in each frontend, on a port of its own. FRONTENDS=vue,svelte picks some.
export const frontends = [
  { name: 'react', flags: [], port: 8201 },
  { name: 'vue', flags: ['-vue'], port: 8202 },
  { name: 'svelte', flags: ['-svelte'], port: 8203 },
].filter((f) => !process.env.FRONTENDS || process.env.FRONTENDS.split(',').includes(f.name))

// appDir is where a frontend's app is: setup.ts makes the apps under
// TUG_E2E_DIR, and sets it for the tests when it's a directory of its own.
export function appDir(frontend: string): string {
  const dir = process.env.TUG_E2E_DIR
  if (!dir) {
    throw new Error('TUG_E2E_DIR is unset: setup.ts sets it, as playwright.config.ts runs it first')
  }
  return path.join(dir, frontend)
}
