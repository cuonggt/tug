import { anotherBrowser, command, dashboard, expect, logIn, registered, test } from './helpers'

test('an admin, whom the admins command makes, has a page of the jobs that failed, which a user may not see', async ({ page }, info) => {
  const user = await registered(page, info)
  await expect(page.getByRole('link', { name: 'Failed jobs' })).toHaveCount(0)
  const res = await page.goto('/admin/failed-jobs')
  expect(res?.status()).toBe(403)
  await expect(page.getByRole('heading', { name: 'Not allowed' })).toBeVisible()
  await expect(page.getByText('you may not see the jobs that failed')).toBeVisible()

  expect(await command(info, 'admins', 'add', user.email)).toBe(`${user.email} is an admin.\n`)
  await page.goto('/dashboard')
  await page.getByRole('link', { name: 'Failed jobs' }).click()
  await expect(page.getByRole('heading', { name: 'Failed jobs' })).toBeVisible()
  await expect(page.getByText('No job has failed for good.')).toBeVisible()
})

test('a user an admin suspends is logged out of the page they have open, told why, and let back in once restored', async ({ page, browser }, info) => {
  const admin = await registered(page, info)
  await command(info, 'admins', 'add', admin.email)
  // Bob, in a browser of his own, on his dashboard, which hears his
  // notifications.
  const bobs = await anotherBrowser(browser, info)
  const bob = await registered(bobs, info)
  const listening = bobs.waitForResponse((r) => r.url().endsWith('/broadcasts'))
  await bobs.goto('/dashboard')
  await listening

  await page.goto('/dashboard')
  await page.getByRole('link', { name: 'Users' }).click()
  await page.getByRole('searchbox', { name: 'Email or name' }).fill(bob.email)
  await page.getByRole('button', { name: 'Find' }).click()
  // Every test's user is an Ann Lee: Bob is the one found.
  await expect(page).toHaveURL(/\?search=/)
  await page.getByRole('button', { name: `Suspend ${bob.name}` }).click()
  await expect(page.getByText(`${bob.name} is suspended, and has been told by mail.`)).toBeVisible()
  await expect(page.getByText('Suspended', { exact: true })).toBeVisible()

  // His page's next request, as the notification rings its bell, ends his
  // login.
  await expect(bobs).toHaveURL(/\/login$/)
  await expect(bobs.getByText("Your account is suspended, so you've been logged out.")).toBeVisible()
  await logIn(bobs, bob)
  await expect(bobs.getByText('this account is suspended')).toBeVisible()

  await page.getByRole('button', { name: `Restore ${bob.name}` }).click()
  await expect(page.getByText(`${bob.name} is restored, and has been told by mail.`)).toBeVisible()
  await logIn(bobs, bob)
  await expect(dashboard(bobs, bob)).toBeVisible()
  await bobs.context().close()
})

test('an admin acts as a user, with a line on each page that says so, and goes back to their own account', async ({ page, browser }, info) => {
  const admin = await registered(page, info)
  await command(info, 'admins', 'add', admin.email)
  const bobs = await anotherBrowser(browser, info)
  const bob = await registered(bobs, info)
  await bobs.context().close()

  await page.goto(`/admin/users?search=${encodeURIComponent(bob.email)}`)
  await page.getByRole('button', { name: `Act as ${bob.name}` }).click()
  const acting = page.getByText(`You're acting as ${bob.name}, ${bob.email}.`)
  await expect(acting).toBeVisible()
  await expect(dashboard(page, bob)).toBeVisible()
  // His pages, as he sees them: no admin's.
  await expect(page.getByRole('link', { name: 'Users' })).toHaveCount(0)
  // What asks for the password again asks for his, which the admin doesn't
  // know, under the same line.
  await page.goto('/settings/security')
  await expect(page).toHaveURL(/\/confirm-password$/)
  await expect(acting).toBeVisible()

  await page.getByRole('button', { name: 'Back to your account' }).click()
  await expect(page).toHaveURL(/\/admin\/users$/)
  await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible()
  await expect(page.getByText("You're acting as")).toHaveCount(0)
})
