import { expect, test } from '@playwright/test'

// The tests share one server, and the last deletes a post.

test('the list shows first, and its stats once counted', async ({ page, request }) => {
  // The first visit's page has the posts, and names the stats for later.
  const html = await (await request.get('/')).text()
  const data = html.match(/<script data-page="app" type="application\/json">(.*?)<\/script>/)
  const first = JSON.parse(data![1])
  expect(first.props.posts).toHaveLength(3)
  expect(first.props.stats).toBeUndefined()
  expect(first.deferredProps).toEqual({ default: ['stats'] })

  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Hello, tug' })).toBeVisible()
  await expect(page.getByTestId('stats')).toContainText('3 posts, 28 words')
})

test('a link changes the page without loading a new one', async ({ page }) => {
  await page.goto('/')
  await page.evaluate(() => Object.assign(window, { stillHere: true }))
  await page.getByRole('link', { name: 'Hello, tug' }).click()

  await expect(page.getByRole('heading', { name: 'Hello, tug' })).toBeVisible()
  await expect(page).toHaveURL(/\/posts\/1$/)
  await expect(page).toHaveTitle('Hello, tug · tug')
  expect(await page.evaluate(() => 'stillHere' in window)).toBe(true)
})

test('recounting asks the server for the stats alone', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByTestId('stats')).toBeVisible()

  const reload = page.waitForResponse((r) => r.request().headers()['x-inertia-partial-data'] === 'stats')
  await page.getByRole('button', { name: 'Recount' }).click()
  const body = await (await reload).json()
  expect(Object.keys(body.props).sort()).toEqual(['errors', 'stats'])
})

test('deleting a post goes back to the list, without it', async ({ page }) => {
  await page.goto('/posts/3')
  await expect(page.getByRole('listitem')).toHaveCount(0) // no tags, and no crash for it
  await page.getByRole('button', { name: 'Delete' }).click()

  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('link', { name: 'Hello, tug' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Delete me' })).toHaveCount(0)
})
