import { dashboard, expect, logIn, logOut, mailedLink, newUser, register, registered, test } from './helpers'

test('someone registers, follows the link mailed to them, and has their dashboard', async ({ page }, info) => {
  const user = newUser()
  await register(page, user)
  await expect(page.getByText(`Welcome, ${user.name}!`)).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Verify your email' })).toBeVisible()
  await expect(page.getByText(user.email)).toBeVisible()

  // The link opens a first page, whose flash shows as a toast as it loads.
  await page.goto(await mailedLink(info, user.email, '/verify-email/'))
  await expect(dashboard(page, user)).toBeVisible()
  await expect(page.getByText('Thanks: your email is verified.')).toBeVisible()
})

test('a taken email is said so as the field is left, before the form is sent', async ({ page }, info) => {
  const user = await registered(page, info)
  await logOut(page)
  await page.goto('/register')
  await page.getByLabel('Email').fill(user.email)
  await page.getByLabel('Email').blur()
  await expect(page.getByText('email has an account already: log in to it')).toBeVisible()
})

test('a user logs out, and in again with their password', async ({ page }, info) => {
  const user = await registered(page, info)
  await logOut(page)
  await expect(page.getByText("You've logged out.")).toBeVisible()
  await logIn(page, user)
  await expect(dashboard(page, user)).toBeVisible()
})

test('a wrong password logs no one in', async ({ page }, info) => {
  const user = await registered(page, info)
  await logOut(page)
  await logIn(page, user, 'wrong horse battery')
  await expect(page.getByText("the email and password don't match an account")).toBeVisible()
  await expect(page).toHaveURL(/\/login$/)
})

test('a forgotten password is set again by the link mailed to the user', async ({ page }, info) => {
  const user = await registered(page, info)
  await logOut(page)
  await page.goto('/login')
  await page.getByRole('link', { name: 'Forgotten it?' }).click()
  // The login page has an Email too, until the visit swaps it out.
  await expect(page.getByRole('heading', { name: 'Forgotten your password?' })).toBeVisible()
  await page.getByLabel('Email').fill(user.email)
  await page.getByRole('button', { name: 'Mail me a link' }).click()
  await expect(page.getByText('If that email has an account, a link to reset its password is on its way.')).toBeVisible()

  await page.goto(await mailedLink(info, user.email, '/reset-password/'))
  await expect(page.getByLabel('Email')).toHaveValue(user.email)
  await page.getByLabel('New password', { exact: true }).fill('battery staple horse')
  await page.getByLabel('New password again').fill('battery staple horse')
  await page.getByRole('button', { name: 'Set the password' }).click()
  await expect(page.getByText('Your password is reset.')).toBeVisible()
  await expect(dashboard(page, user)).toBeVisible()

  await logOut(page)
  await logIn(page, user, 'battery staple horse')
  await expect(dashboard(page, user)).toBeVisible()
})

test("a page that isn't there is the error page, with its status", async ({ page }) => {
  const res = await page.goto('/nowhere')
  expect(res?.status()).toBe(404)
  await expect(page.getByRole('heading', { name: 'Not found' })).toBeVisible()
  await page.getByRole('link', { name: 'Back home' }).click()
  await expect(page).toHaveURL(/\/$/)
})
