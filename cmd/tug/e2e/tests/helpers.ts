import { execFile } from 'node:child_process'
import { createHmac, randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { promisify } from 'node:util'
import { test as base, expect, type Page, type TestInfo } from '@playwright/test'
import { appDir, dotEnv, frontends } from '../apps'

export { expect }

// test is Playwright's, failing a test whose page says anything in the
// console: a hydration mismatch, an error thrown, or a warning. A resource
// that didn't load is left out, as Precognition's 422s are ones.
export const test = base.extend<{ quietConsole: void }>({
  quietConsole: [
    async ({ page }, use) => {
      const said: string[] = []
      page.on('console', (m) => {
        if ((m.type() === 'error' || m.type() === 'warning') && !m.text().startsWith('Failed to load resource')) {
          said.push(`${m.type()}: ${m.text()}`)
        }
      })
      page.on('pageerror', (err) => said.push(`thrown: ${err.message}`))
      await use()
      expect(said, 'what the page said in the console').toEqual([])
    },
    { auto: true },
  ],
})

// A User is someone new to the app: each test has its own, as the apps'
// databases last the whole run.
export type User = { name: string; email: string; password: string }

export function newUser(): User {
  return { name: 'Ann Lee', email: `ann.${randomUUID().slice(0, 8)}@example.com`, password: 'correct horse battery' }
}

// register makes an account with the form, which logs the user in, and
// leaves them where the app asks them to verify their email.
export async function register(page: Page, user: User) {
  await page.goto('/register')
  await page.getByLabel('Name').fill(user.name)
  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Password', { exact: true }).fill(user.password)
  await page.getByLabel('Password again').fill(user.password)
  await page.getByRole('button', { name: 'Register' }).click()
  await expect(page).toHaveURL(/\/verify-email$/)
}

// registered is a new user who has registered and followed the link mailed
// to them, on their dashboard.
export async function registered(page: Page, info: TestInfo): Promise<User> {
  const user = newUser()
  await register(page, user)
  await page.goto(await mailedLink(info, user.email, '/verify-email/'))
  await expect(dashboard(page, user)).toBeVisible()
  return user
}

// dashboard is the heading that greets the user on their dashboard.
export function dashboard(page: Page, user: User) {
  return page.getByRole('heading', { name: `Hello, ${user.name}` })
}

export async function logIn(page: Page, user: User, password = user.password) {
  await page.goto('/login')
  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Log in', exact: true }).click()
}

export async function logOut(page: Page) {
  await page.getByRole('button', { name: 'Your account' }).click()
  await page.getByRole('menuitem', { name: 'Log out' }).click()
  await expect(page).toHaveURL(/\/$/)
}

// mailedLink is the link in the last mail to email whose link has path in
// it, which the app writes to its log, as it does without a MAIL_HOST. The
// mail goes by a background job, so it waits for it a while.
export async function mailedLink(info: TestInfo, email: string, path_: string): Promise<string> {
  const log = path.join(appDir(info.project.name), 'app.log')
  for (const deadline = Date.now() + 10_000; Date.now() < deadline; await new Promise((r) => setTimeout(r, 100))) {
    const mails = (await readFile(log, 'utf8')).split('mail, not sent').filter((m) => m.includes(`To: ${email}\n`))
    for (const mail of mails.reverse()) {
      const link = mail.match(/https?:\/\/\S+/g)?.find((l) => l.includes(path_))
      if (link) return link
    }
  }
  throw new Error(`no mail to ${email} with a link to ${path_} in ${log}`)
}

// command runs one of the app's commands, as its binary does in place of
// serving, as ./app admins add ann@example.com, in the app's directory
// with its .env and the APP_URL it serves at, as the app needs to start,
// and returns what it printed.
export async function command(info: TestInfo, ...args: string[]): Promise<string> {
  const dir = appDir(info.project.name)
  const port = frontends.find((f) => f.name === info.project.name)?.port
  const env = { ...process.env, ...dotEnv(await readFile(path.join(dir, '.env'), 'utf8')), APP_URL: `http://localhost:${port}` }
  const { stdout } = await promisify(execFile)(path.join(dir, 'app'), args, { cwd: dir, env })
  return stdout
}

// totp is the code an authenticator app shows for the key at a time: HOTP
// (RFC 4226) of the 30-second steps since 1970, as package auth makes it.
export function totp(key: string, at = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = 0
  let value = 0
  const secret: number[] = []
  for (const c of key.replace(/[\s=]/g, '').toUpperCase()) {
    value = (value << 5) | alphabet.indexOf(c)
    bits += 5
    if (bits >= 8) {
      secret.push((value >>> (bits - 8)) & 0xff)
      bits -= 8
    }
  }
  const step = Buffer.alloc(8)
  step.writeBigUInt64BE(BigInt(Math.floor(at / 30_000)))
  const mac = createHmac('sha1', Buffer.from(secret)).update(step).digest()
  const offset = mac[mac.length - 1] & 0xf
  return String((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

// authenticator gives the page's browser a passkey authenticator in
// software, Chrome's, as a phone with a fingerprint reader: it keeps
// passkeys, and says yes to each, the user verified, with no prompt.
export async function authenticator(page: Page) {
  const cdp = await page.context().newCDPSession(page)
  await cdp.send('WebAuthn.enable')
  await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
      automaticPresenceSimulation: true,
    },
  })
}
