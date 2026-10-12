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
    expect(headers['content-security-policy']).toMatch(/style-src 'nonce-[A-Z2-7]+' 'self'; style-src-attr 'unsafe-inline'/)
    expect(headers['content-security-policy-report-only']).toBeUndefined()
    expect(headers['permissions-policy']).toMatch(/^camera=\(\), microphone=\(\), geolocation=\(\), /)
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

test('a style the page did not bring is blocked, and the browser reports it to the app', async ({ context }, info) => {
  const tab = await context.newPage()
  const where = `/nowhere-${randomUUID()}`
  await tab.goto(where)
  const hidden = await tab.evaluate(() => {
    const style = document.createElement('style')
    style.textContent = 'body { display: none }'
    document.head.append(style)
    return getComputedStyle(document.body).display === 'none'
  })
  expect(hidden).toBe(false)

  const log = path.join(appDir(info.project.name), 'app.log')
  const reported = async () =>
    (await readFile(log, 'utf8'))
      .split('\n')
      .some((line) => line.includes('blocked what it asked for') && line.includes(where) && line.includes('style-src'))
  await expect.poll(reported).toBe(true)
})

test("a visit's progress bar, and a toast, are styled, as the page's nonce lets their styles in", async ({ page }) => {
  await page.goto('/login')
  // Inertia's progress bar puts its <style> in the head as the app starts,
  // with the nonce it was handed: a style the policy blocked has no sheet.
  const progress = await page.evaluate(() =>
    [...document.querySelectorAll('style')].some((s) => s.textContent?.includes('#nprogress') && s.sheet !== null),
  )
  expect(progress).toBe(true)
  await page.getByRole('link', { name: 'Forgotten it?' }).click()
  await page.getByLabel('Email').fill('nobody@example.com')
  await page.getByRole('button', { name: 'Mail me a link' }).click()
  const toast = page.locator('[data-sonner-toast]').first()
  await expect(toast).toBeVisible()
  // sonner's stylesheet, by its own rules, in place of the <style> it injects.
  expect(await toast.evaluate((el) => getComputedStyle(el).position)).toBe('absolute')
})
