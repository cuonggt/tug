import { expect, logIn, registered, test } from './helpers'

test("a new name is saved, and the header's menu has it", async ({ page }, info) => {
  await registered(page, info)
  await page.goto('/settings/profile')
  await page.getByLabel('Name').fill('Ann Smith')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Profile saved.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Your account' })).toContainText('Ann Smith')
})

test('the appearance chosen is kept by the browser, and applied before the page paints', async ({ page }, info) => {
  await registered(page, info)
  await page.goto('/settings/appearance')
  const html = page.locator('html')
  await page.getByRole('radio', { name: 'Dark' }).click()
  await expect(page.getByRole('radio', { name: 'Dark' })).toBeChecked()
  await expect(html).toHaveClass(/\bdark\b/)

  await page.reload()
  await expect(html).toHaveClass(/\bdark\b/)
  await expect(page.getByRole('radio', { name: 'Dark' })).toBeChecked()

  await page.getByRole('radio', { name: 'Light' }).click()
  await expect(html).not.toHaveClass(/\bdark\b/)
})

test('deleting the account asks for the password, and logs the user out for good', async ({ page }, info) => {
  const user = await registered(page, info)
  await page.goto('/settings/profile')
  await page.getByRole('button', { name: 'Delete my account' }).click()
  const dialog = page.getByRole('dialog', { name: 'Delete your account?' })
  await dialog.getByLabel('Password', { exact: true }).fill('wrong horse battery')
  await dialog.getByRole('button', { name: 'Delete my account' }).click()
  await expect(dialog.getByText("that isn't your password")).toBeVisible()

  await dialog.getByLabel('Password', { exact: true }).fill(user.password)
  await dialog.getByRole('button', { name: 'Delete my account' }).click()
  await expect(page.getByText('Your account is deleted.')).toBeVisible()
  await expect(page).toHaveURL(/\/$/)

  await logIn(page, user)
  await expect(page.getByText("the email and password don't match an account")).toBeVisible()
})
