import { expect, registered, test } from './helpers'

test('an API token made in the settings calls the API in place of a login, until it is revoked', async ({ page, request }, info) => {
  const user = await registered(page, info)
  await page.goto('/settings/tokens')
  await page.getByLabel('Name').fill('My script')
  await page.getByLabel('user:read').check()
  await page.getByLabel('Expires').selectOption('365')
  await page.getByRole('button', { name: 'Make a token' }).click()
  const token = await page.getByTestId('new-token').innerText()
  expect(token).toMatch(/^[a-z0-9]+_[a-z2-7]{52}$/)

  // The token alone, with none of the page's cookies, as a script sends it.
  const call = () => request.get('/api/user', { headers: { Authorization: `Bearer ${token}`, Accept: 'application/json' } })
  const answer = await call()
  expect(answer.status()).toBe(200)
  expect((await answer.json()).email).toBe(user.email)

  // It's shown once, and listed after, with its use.
  await page.reload()
  await expect(page.getByTestId('new-token')).toHaveCount(0)
  await expect(page.getByText('My script')).toBeVisible()
  await expect(page.getByText(/last used/)).toBeVisible()

  await page.getByRole('button', { name: 'Revoke My script' }).click()
  await expect(page.getByText('Token revoked.')).toBeVisible()
  expect((await call()).status()).toBe(401)
})
