import type { Page } from '@playwright/test'
import { authenticator, dashboard, expect, logIn, logOut, madeToken, registered, test, totp } from './helpers'

test('two-factor logins go on with a code from the app, and then each login asks for one', async ({ page }, info) => {
  const user = await registered(page, info)
  await page.goto('/settings/security')
  await page.getByRole('button', { name: 'Turn two-factor logins on' }).click()

  // The key an authenticator app would scan, which the page also shows in
  // fours, to type.
  const key = await page.getByText(/^[A-Z2-7]{4}( [A-Z2-7]{1,4})+$/).innerText()
  await page.getByLabel('The code from the app').fill(totp(key))
  await page.getByRole('button', { name: 'Turn on' }).click()
  await expect(page.getByText('Two-factor logins are on.')).toBeVisible()
  const codes = page.getByText('Keep these somewhere safe').locator('xpath=following-sibling::ul[1]/li')
  await expect(codes).toHaveCount(8)
  const recovery = await codes.first().innerText()
  // A change in another tab rings the bell, whose reload empties the flash
  // the codes came in: they're still shown.
  await madeToken(await page.context().newPage(), 'My script')
  await expect(page.getByRole('link', { name: 'Notifications, 1 unread' })).toBeVisible()
  await expect(codes).toHaveCount(8)

  // A code works once, so the one that turned them on can't log in: the
  // next one does, which the server takes 30 seconds early.
  await logOut(page)
  await logIn(page, user)
  await expect(page).toHaveURL(/\/two-factor-challenge$/)
  await page.getByLabel('Code', { exact: true }).fill(totp(key, Date.now() + 30_000))
  await page.getByRole('button', { name: 'Log in' }).click()
  await expect(dashboard(page, user)).toBeVisible()

  // With the phone lost, a recovery code logs in once.
  await logOut(page)
  await logIn(page, user)
  await page.getByRole('button', { name: 'Use a recovery code' }).click()
  await page.getByLabel('Recovery code').fill(recovery)
  await page.getByRole('button', { name: 'Log in' }).click()
  await expect(dashboard(page, user)).toBeVisible()

  await page.goto('/settings/security')
  await page.getByRole('button', { name: 'Turn two-factor logins off' }).click()
  await expect(page.getByText('Two-factor logins are off.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Turn two-factor logins on' })).toBeVisible()
})

test('a passkey is added, and logs in from the email field, with no password', async ({ page }, info) => {
  await authenticator(page)
  const user = await registered(page, info)
  await addPasskey(page, 'My phone')

  // The login page asks for a passkey as its email field is focused, which
  // a browser offers in the field's autofill: the virtual authenticator
  // picks the one it has at once.
  await logOut(page)
  await page.goto('/login')
  await expect(dashboard(page, user)).toBeVisible()
})

test('with no passkey autofill, the button logs in with a passkey, which is then removed', async ({ page }, info) => {
  await page.addInitScript(() => {
    PublicKeyCredential.isConditionalMediationAvailable = async () => false
  })
  await authenticator(page)
  const user = await registered(page, info)
  await addPasskey(page, 'My phone')

  await logOut(page)
  await page.goto('/login')
  await page.getByRole('button', { name: 'Log in with a passkey' }).click()
  await expect(dashboard(page, user)).toBeVisible()

  await page.goto('/settings/security')
  await page.getByRole('button', { name: 'Remove My phone' }).click()
  await expect(page.getByText('Passkey removed.')).toBeVisible()
  await expect(page.getByText('My phone')).toHaveCount(0)
})

async function addPasskey(page: Page, name: string) {
  await page.goto('/settings/security')
  await page.getByRole('button', { name: 'Add a passkey' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add a passkey' })
  await dialog.getByLabel('Name').fill(name)
  await dialog.getByRole('button', { name: 'Add', exact: true }).click()
  await expect(page.getByText('Passkey added: it logs you in with no password.')).toBeVisible()
  await expect(dialog).toBeHidden()
  await expect(page.getByText(name)).toBeVisible()
}

test('a new password logs in, and the old one no longer does', async ({ page }, info) => {
  const user = await registered(page, info)
  await page.goto('/settings/security')
  await page.getByLabel('Current password').fill(user.password)
  await page.getByLabel('New password', { exact: true }).fill('battery staple horse')
  await page.getByLabel('New password again').fill('battery staple horse')
  await page.getByRole('button', { name: 'Change the password' }).click()
  await expect(page.getByText("Password changed. You've been logged out everywhere else.")).toBeVisible()

  await logOut(page)
  await logIn(page, user)
  await expect(page.getByText("the email and password don't match an account")).toBeVisible()
  await logIn(page, user, 'battery staple horse')
  await expect(dashboard(page, user)).toBeVisible()
})
