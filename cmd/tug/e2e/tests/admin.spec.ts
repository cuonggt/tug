import { command, expect, registered, test } from './helpers'

test('an admin, whom the admins command makes, has a page of the jobs that failed, which a user may not see', async ({ page }, info) => {
  const user = await registered(page, info)
  await expect(page.getByRole('link', { name: 'Failed jobs' })).toHaveCount(0)
  const res = await page.goto('/admin/failed-jobs')
  expect(res?.status()).toBe(403)
  await expect(page.getByRole('heading', { name: 'Not allowed' })).toBeVisible()
  await expect(page.getByText('you may not see the jobs that failed')).toBeVisible()

  expect(await command(info, 'admins', 'add', user.email)).toBe(`${user.email} is an admin.\n`)
  await page.goto('/dashboard')
  await page.getByRole('link', { name: 'Failed jobs' }).click()
  await expect(page.getByRole('heading', { name: 'Failed jobs' })).toBeVisible()
  await expect(page.getByText('No job has failed for good.')).toBeVisible()
})
