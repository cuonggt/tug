import { expect, registered, test } from './helpers'

test('a change made in another tab rings the bell at once, and the list shows it read', async ({ page, context }, info) => {
  await registered(page, info)
  const listening = page.waitForResponse((r) => r.url().endsWith('/broadcasts'))
  await page.goto('/dashboard')
  await listening
  await expect(page.getByRole('link', { name: 'Notifications', exact: true })).toBeVisible()

  const other = await context.newPage()
  await other.goto('/settings/tokens')
  await other.getByLabel('Name').fill('My script')
  await other.getByLabel('user:read').check()
  await other.getByRole('button', { name: 'Make a token' }).click()
  await expect(other.getByTestId('new-token')).toBeVisible()

  const bell = page.getByRole('link', { name: 'Notifications, 1 unread' })
  await expect(bell).toBeVisible()
  await bell.click()
  await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible()
  await expect(page.getByText('An API token, My script, was made.')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Notifications', exact: true })).toBeVisible()
})
