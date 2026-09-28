import { execFile, spawn, type ChildProcess } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, open, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { promisify } from 'node:util'
import { appDir, frontends } from './apps'

const exec = promisify(execFile)
const repo = path.resolve(import.meta.dirname, '../../..')

// setup makes an app with accounts in each frontend, as a person would with
// tug new, and runs it as it runs deployed: its frontend built, its pages
// rendered on the server by Node, and its mail written to its log, where
// the tests find the links in it. It returns what stops them.
//
// The apps are made in a new directory each run. TUG_E2E_DIR names one to
// keep them in instead, and an app already made there is run as it is:
// delete it to make it again, after a change to the starters.
export default async function setup() {
  process.env.TUG_E2E_DIR ??= await mkdtemp(path.join(tmpdir(), 'tug-e2e-'))
  const tug = path.join(process.env.TUG_E2E_DIR, 'tug')
  await sh(repo, 'go', 'build', '-o', tug, './cmd/tug')
  await Promise.all(frontends.map((f) => make(tug, appDir(f.name), f.flags)))
  const apps = await Promise.all(frontends.map((f) => serve(appDir(f.name), f.port)))
  return async () => {
    await Promise.all(apps.map((app) => stop(app)))
  }
}

async function make(tug: string, dir: string, flags: string[]) {
  if (existsSync(path.join(dir, 'app'))) {
    return
  }
  await rm(dir, { recursive: true, force: true })
  await sh(repo, tug, 'new', '-tug-dir', repo, '-auth', '-ssr', ...flags, dir)
  await sh(dir, 'npm', 'run', 'build')
  await sh(dir, 'go', 'build', '-o', 'app', '.')
}

// serve runs the app in dir on port, with its .env, its output in app.log,
// and APP_URL at localhost, where browsers make passkeys, as they don't for
// an IP address. It returns once the app answers its health check.
async function serve(dir: string, port: number): Promise<ChildProcess> {
  const log = await open(path.join(dir, 'app.log'), 'a')
  const env = {
    ...process.env,
    ...dotEnv(await readFile(path.join(dir, '.env'), 'utf8')),
    ADDR: `127.0.0.1:${port}`,
    APP_URL: `http://localhost:${port}`,
  }
  const app = spawn(path.join(dir, 'app'), { cwd: dir, env, stdio: ['ignore', log.fd, log.fd] })
  await log.close()
  for (const deadline = Date.now() + 60_000; ; await new Promise((r) => setTimeout(r, 200))) {
    if (app.exitCode !== null) {
      throw new Error(`the app in ${dir} stopped as it started: see its app.log`)
    }
    if (Date.now() > deadline) {
      await stop(app)
      throw new Error(`the app in ${dir} didn't answer on port ${port} within a minute: see its app.log`)
    }
    const up = await fetch(`http://127.0.0.1:${port}/up`).catch(() => null)
    if (up?.ok) {
      return app
    }
  }
}

async function stop(app: ChildProcess) {
  if (app.exitCode === null) {
    const exited = new Promise((r) => app.once('exit', r))
    app.kill('SIGTERM')
    await exited
  }
}

// dotEnv reads the variables tug new wrote to .env, which are plain
// NAME=value lines.
function dotEnv(text: string): Record<string, string> {
  const vars: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const m = line.match(/^\s*([A-Z_][A-Z0-9_]*)\s*=\s*(.*?)\s*$/)
    if (m) vars[m[1]] = m[2]
  }
  return vars
}

async function sh(cwd: string, cmd: string, ...args: string[]) {
  try {
    await exec(cmd, args, { cwd, maxBuffer: 64 << 20 })
  } catch (err) {
    const { stdout, stderr } = err as { stdout?: string; stderr?: string }
    throw new Error(`${cmd} ${args.join(' ')}, in ${cwd}: ${err}\n${stdout ?? ''}${stderr ?? ''}`)
  }
}
