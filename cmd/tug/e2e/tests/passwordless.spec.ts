import type { Page } from '@playwright/test'
import { authenticator, dashboard, expect, logIn, logOut, mailedLink, registeredWithPasskey, test } from './helpers'

// noAutofill keeps the login page from logging in with the passkey as its
// email field is focused, as the virtual authenticator picks one at once,
// for a test of the password's.
async function noAutofill(page: Page) {
  await page.addInitScript(() => {
    PublicKeyCredential.isConditionalMediationAvailable = async () => false
  })
}

test('someone registers with a passkey from the link mailed to them, and logs in with it, with no password', async ({ page }, info) => {
  await authenticator(page)
  const user = await registeredWithPasskey(page, info)
  await expect(page.getByText(`Welcome, ${user.name}!`)).toBeVisible()
  await page.goto('/settings/security')
  await expect(page.getByLabel('Current password')).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Set a password' })).toBeVisible()

  await logOut(page)
  await page.goto('/login')
  await expect(dashboard(page, user)).toBeVisible()
})

test('an account with no password sets one, and removes it again, with a passkey to log in with', async ({ page }, info) => {
  await noAutofill(page)
  await authenticator(page)
  const user = await registeredWithPasskey(page, info)
  await page.goto('/settings/security')
  await page.getByLabel('New password', { exact: true }).fill(user.password)
  await page.getByLabel('New password again').fill(user.password)
  await page.getByRole('button', { name: 'Set a password' }).click()
  await expect(page.getByText('Password set: it logs you in too.')).toBeVisible()

  await logOut(page)
  await logIn(page, user)
  await expect(dashboard(page, user)).toBeVisible()

  await page.goto('/settings/security')
  await page.getByRole('button', { name: 'Remove the password' }).click()
  await expect(page.getByText('Password removed: your passkeys log you in.')).toBeVisible()
  await logOut(page)
  await logIn(page, user)
  await expect(page.getByText("the email and password don't match an account")).toBeVisible()
  await page.getByRole('button', { name: 'Log in with a passkey' }).click()
  await expect(dashboard(page, user)).toBeVisible()
})

test('an account whose passkey is lost is back in by a reset link, which sets a password', async ({ page }, info) => {
  await noAutofill(page)
  await authenticator(page)
  const user = await registeredWithPasskey(page, info)
  await logOut(page)
  await page.goto('/login')
  await page.getByRole('link', { name: 'Forgotten it?' }).click()
  await expect(page.getByRole('heading', { name: 'Forgotten your password, or lost your passkey?' })).toBeVisible()
  await page.getByLabel('Email').fill(user.email)
  await page.getByRole('button', { name: 'Mail me a link' }).click()
  await expect(page.getByText('If that email has an account')).toBeVisible()

  await page.goto(await mailedLink(info, user.email, '/reset-password/'))
  await page.getByLabel('New password', { exact: true }).fill(user.password)
  await page.getByLabel('New password again').fill(user.password)
  await page.getByRole('button', { name: 'Set the password' }).click()
  await expect(page.getByText('Your password is set.')).toBeVisible()
  await expect(dashboard(page, user)).toBeVisible()

  await logOut(page)
  await logIn(page, user)
  await expect(dashboard(page, user)).toBeVisible()
})
