import { randomUUID } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import path from 'node:path'
import type { Response } from '@playwright/test'
import { appDir } from '../apps'
import { expect, test } from './helpers'

test('a page, and an error page, say what a browser may do with them', async ({ page }) => {
  for (const [url, status] of [['/login', 200], ['/nowhere', 404]] as const) {
    const response = await page.goto(url)
    expect(response?.status()).toBe(status)
    const headers = response!.headers()
    expect(headers['x-content-type-options']).toBe('nosniff')
    expect(headers['referrer-policy']).toBe('strict-origin-when-cross-origin')
    expect(headers['cross-origin-opener-policy']).toBe('same-origin')
    expect(headers['x-frame-options']).toBe('SAMEORIGIN')
    // The apps here are served over plain HTTP, with no HTTPS to hold to.
    expect(headers['strict-transport-security']).toBeUndefined()
    expect(headers['content-security-policy']).toMatch(/script-src 'nonce-[A-Z2-7]+' 'strict-dynamic'/)
    expect(headers['content-security-policy-report-only']).toBeUndefined()
  }
})

test("the build's scripts and styles come gzipped, and the page works", async ({ page }) => {
  const assets: Response[] = []
  page.on('response', (r) => {
    if (/^\/build\/assets\/[^/]+\.(js|css)$/.test(new URL(r.url()).pathname)) assets.push(r)
  })
  await page.goto('/login')
  await expect(page.getByRole('button', { name: 'Log in', exact: true })).toBeVisible()
  expect(assets.length).toBeGreaterThan(0)
  for (const r of assets) {
    // One so small that gzip would make it longer goes as it is.
    if ((await r.body()).length > 1024) {
      expect(r.headers()['content-encoding'], new URL(r.url()).pathname).toBe('gzip')
    }
  }
})

test('a script the page did not bring is blocked, and the browser reports it to the app', async ({ context }, info) => {
  // A tab of its own, as the browser says what it blocked in the console,
  // which every other test's page keeps quiet.
  const tab = await context.newPage()
  const where = `/nowhere-${randomUUID()}`
  await tab.goto(where)
  const ran = await tab.evaluate(() => {
    const button = document.createElement('button')
    button.setAttribute('onclick', 'window.ran = true')
    document.body.append(button)
    button.click()
    return 'ran' in window
  })
  expect(ran).toBe(false)

  const log = path.join(appDir(info.project.name), 'app.log')
  const reported = async () =>
    (await readFile(log, 'utf8')).split('\n').some((line) => line.includes('blocked what it asked for') && line.includes(where))
  await expect.poll(reported).toBe(true)
})
