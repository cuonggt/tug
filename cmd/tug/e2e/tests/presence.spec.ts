import { expect, logIn, registered, test } from './helpers'

// Every page of the app is on the user's own channel while it's open,
// which is how the security page says which of their browsers has one
// open: the hub says presence.joined and presence.left as a page comes or
// goes, and the list reloads.
test('the security page says which browsers are open, as they come and go', async ({ page, browser }, info) => {
  const user = await registered(page, info)
  await page.goto('/settings/security')
  const open = page.getByText('open now')
  await expect(open).toHaveCount(1)

  // Another browser of hers, logged in to the same account, with a page
  // open on it: this one says so without being asked.
  const other = await browser.newContext()
  const phone = await other.newPage()
  await logIn(phone, user)
  await expect(open).toHaveCount(2)

  // And as that page goes, it says that too.
  await other.close()
  await expect(open).toHaveCount(1)
})
