import { expect, test } from './helpers'

// The apps know Arabic, from the empty lang/ar.json setup.ts writes: its
// words are English's until there are any, and a language is written the
// way it's written either way. A browser that asks for Arabic gets the
// page in it, as c.Locale picks the language from Accept-Language, and the
// document around the page says which way it's written.
test.use({ locale: 'ar' })

test('a page in a right-to-left language says so, and is laid out that way', async ({ page }) => {
  await page.goto('/login')
  await expect(page.locator('html')).toHaveAttribute('lang', 'ar')
  await expect(page.locator('html')).toHaveAttribute('dir', 'rtl')

  // The button that shows the password sits at the end of the field,
  // which in Arabic is its left: the utility that puts it there names the
  // end, not the right, so it mirrors itself. The rest of this suite sees
  // the other side, as it asks in English.
  const field = await page.getByLabel('Password', { exact: true }).boundingBox()
  const button = await page.getByRole('button', { name: 'Show the password' }).boundingBox()
  expect(button).not.toBeNull()
  expect(button!.x + button!.width / 2).toBeLessThan(field!.x + field!.width / 2)
})
