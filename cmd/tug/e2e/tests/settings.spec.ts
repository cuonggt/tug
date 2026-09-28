import { expect, logIn, registered, test } from './helpers'

test("a new name is saved, and the header's menu has it", async ({ page }, info) => {
  await registered(page, info)
  await page.goto('/settings/profile')
  await page.getByLabel('Name').fill('Ann Smith')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByText('Profile saved.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Your account' })).toContainText('Ann Smith')
})

// pixel is a PNG of one pixel, which a browser shows.
const pixel = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
  'base64',
)

test('a photo is uploaded, shown in place of the initials by a signed link, and removed', async ({ page }, info) => {
  await registered(page, info)
  await page.goto('/settings/profile')
  // The initials the avatar shows, as it hides them rather than removes
  // them in some frontends.
  const menu = page.getByRole('button', { name: 'Your account' })
  const initials = { useInnerText: true }
  await expect(menu).toContainText('AL', initials)

  // A page, whatever it's called, isn't a photo.
  const choose = page.getByLabel('Choose a photo')
  await choose.setInputFiles({ name: 'ann.png', mimeType: 'image/png', buffer: Buffer.from('<!DOCTYPE html><p>not a photo') })
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText('photo must be a PNG, JPEG or WebP image')).toBeVisible()

  await choose.setInputFiles({ name: 'ann.png', mimeType: 'image/png', buffer: pixel })
  await page.getByRole('button', { name: 'Upload' }).click()
  await expect(page.getByText('Photo saved.')).toBeVisible()
  const photo = menu.locator('img')
  await expect(photo).toHaveAttribute('src', /^\/files\/photos\/[a-z2-7]{26}\.png\?expires=\d+&signature=/)
  await expect(photo).toHaveJSProperty('naturalWidth', 1)
  await expect(menu).not.toContainText('AL', initials)
  await expect(page.getByRole('img', { name: 'Your photo' })).toBeVisible()

  await page.getByRole('button', { name: 'Remove' }).click()
  await expect(page.getByText('Photo removed.')).toBeVisible()
  await expect(photo).toHaveCount(0)
  await expect(menu).toContainText('AL', initials)
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
