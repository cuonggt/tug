import { expect, test } from './helpers'

test('the home page has the head its handler gives it, once, and another page has its own', async ({ page }) => {
  const description = page.locator('meta[name="description"]')
  const home = async () => {
    await expect(page).toHaveTitle(/^Welcome · /)
    await expect(description).toHaveCount(1)
    await expect(description).toHaveAttribute('content', /^Go handlers render .+'s pages, with Inertia in between/)
  }

  await page.goto('/')
  await home()

  // A page whose handler gives it none has its <Head>'s title, and no
  // description.
  await page.getByRole('link', { name: 'Log in' }).click()
  await expect(page).toHaveURL(/\/login$/)
  await expect(page).toHaveTitle(/^Log in · /)
  await expect(description).toHaveCount(0)

  await page.goBack()
  await expect(page).toHaveURL(/\/$/)
  await home()
})
