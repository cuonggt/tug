import { expect, madeToken, registered, test } from './helpers'

test('a change made in another tab rings the bell at once, and the list shows it read', async ({ page, context }, info) => {
  await registered(page, info)
  const listening = page.waitForResponse((r) => r.url().endsWith('/broadcasts'))
  await page.goto('/dashboard')
  await listening
  await expect(page.getByRole('link', { name: 'Notifications', exact: true })).toBeVisible()

  await madeToken(await context.newPage(), 'My script')

  const bell = page.getByRole('link', { name: 'Notifications, 1 unread' })
  await expect(bell).toBeVisible()
  await bell.click()
  await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible()
  await expect(page.getByText('An API token, My script, was made.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Notifications', exact: true })).toBeVisible()
})

// The bell's reload is a visit, and empties the page's flash, which a new
// token comes in, once.
test('a new token is still shown as a change in another tab rings the bell', async ({ page, context }, info) => {
  await registered(page, info)
  const listening = page.waitForResponse((r) => r.url().endsWith('/broadcasts'))
  const token = await madeToken(page, 'My script')
  await listening

  await madeToken(await context.newPage(), 'Another script')
  await expect(page.getByRole('link', { name: 'Notifications, 2 unread' })).toBeVisible()
  await expect(page.getByTestId('new-token')).toHaveText(token)
})

// The bell waits for a request of Inertia's in flight, as the visit of a
// form that changed the account and is loading the page it goes back to:
// the two at once would read the flash the form left, and each show it,
// two toasts of one message.
test('the bell rings once the visit in flight is done', async ({ page, context }, info) => {
  await registered(page, info)
  const listening = page.waitForResponse((r) => r.url().endsWith('/broadcasts'))
  await page.goto('/dashboard')
  await listening
  const reloads: string[] = []
  page.on('request', (r) => {
    if (r.headers()['x-inertia-partial-data']) reloads.push(r.url())
  })
  // Inertia says a visit has started, as its own events do, its progress
  // bar reading the visit, and hasn't said it's done.
  await page.evaluate(() => document.dispatchEvent(new CustomEvent('inertia:start', { detail: { visit: { showProgress: true } } })))

  const other = await context.newPage()
  await madeToken(other, 'My script')
  await expect(other.getByRole('link', { name: 'Notifications, 1 unread' })).toBeVisible()
  // The other tab has heard the change; this one has too, and waits.
  await page.waitForTimeout(500)
  expect(reloads).toEqual([])
  await expect(page.getByRole('link', { name: 'Notifications', exact: true })).toBeVisible()

  await page.evaluate(() => document.dispatchEvent(new CustomEvent('inertia:finish', { detail: { visit: { completed: true } } })))
  await expect(page.getByRole('link', { name: 'Notifications, 1 unread' })).toBeVisible()
  expect(reloads).toHaveLength(1)
})
