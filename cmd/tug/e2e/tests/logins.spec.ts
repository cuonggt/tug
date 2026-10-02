import { dashboard, expect, logIn, registered, test } from './helpers'

test('a login from another browser is told of, listed, and logged out from the first', async ({ page, browser }, info) => {
  const user = await registered(page, info)
  const listening = page.waitForResponse((r) => r.url().endsWith('/broadcasts'))
  await page.goto('/dashboard')
  await listening

  // Another browser, with cookies of its own, which the account hasn't
  // logged in from.
  const other = await browser.newContext()
  const theirs = await other.newPage()
  await logIn(theirs, user)
  await expect(dashboard(theirs, user)).toBeVisible()
  await expect(page.getByRole('link', { name: 'Notifications, 1 unread' })).toBeVisible()

  // The first lists both, itself first, and logs the other out.
  await page.goto('/settings/security')
  await expect(page.getByRole('heading', { name: 'Browsers' })).toBeVisible()
  await expect(page.getByText('This browser')).toBeVisible()
  const logOut = page.getByRole('button', { name: /^Log out of (?!every)/ })
  await expect(logOut).toHaveCount(1)
  await logOut.click()
  await expect(page.getByText(/^Logged out of /)).toBeVisible()
  await expect(logOut).toHaveCount(0)

  // Its next visit finds it logged out.
  await theirs.goto('/dashboard')
  await expect(theirs).toHaveURL(/\/login$/)
  await other.close()
})
