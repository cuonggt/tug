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

// A change made in this tab is a notification this tab hears too, as the
// form's visit loads the page it goes back to: Inertia's client refreshes
// a live prop a request in flight has once that request is done, and a
// partial reload leaves the flash to the visit it was left for, so the
// form's flash shows once, one toast of one message.
test('a change made in this tab rings its own bell, and its flash shows once', async ({ page }, info) => {
  await registered(page, info)
  const token = await madeToken(page, 'My script')
  await expect(page.getByRole('link', { name: 'Notifications, 1 unread' })).toBeVisible()
  await expect(page.getByTestId('new-token')).toHaveText(token)
  await expect(page.getByText('Token made. Copy it now')).toHaveCount(1)
})
