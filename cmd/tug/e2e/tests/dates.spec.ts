import { expect, madeToken, registered, test } from './helpers'

// The apps name no APP_TIMEZONE, so the app's own zone is UTC, and that's
// what Node renders a first visit's time in: a server can't know which
// zone the browser is in. The page says the time again in the reader's own
// once it's there, after the markup the browser hydrates, which a time in
// that zone would not have matched: the console this suite watches fails a
// test whose page says a hydration mismatch.
test.use({ timezoneId: 'Asia/Ho_Chi_Minh' })

test("a time is in the reader's own zone, which it says", async ({ page }, info) => {
  await registered(page, info)
  await madeToken(page, 'My script')

  await page.goto('/notifications')
  await expect(page.getByText('An API token, My script, was made.')).toBeVisible()
  // Ho Chi Minh's own name for its zone, where the server said UTC.
  await expect(page.getByText(/GMT\+7$/)).toBeVisible()
  await expect(page.getByText(/UTC$/)).toHaveCount(0)
})
