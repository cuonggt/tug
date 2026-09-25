import { expect, test } from '@playwright/test'

// The tests share one server and run in order: the first ones need the
// three posts the example starts with, and the later ones add, change and
// delete posts.

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

test('a form sent incomplete comes back with what to fix', async ({ page }) => {
  await page.goto('/posts/create')
  await page.getByLabel('Title').fill('Half a post')
  await page.getByRole('button', { name: 'Create post' }).click()

  await expect(page.getByText('body is required')).toBeVisible()
  await expect(page).toHaveURL(/\/posts\/create$/)
  // What was typed is still there.
  await expect(page.getByLabel('Title')).toHaveValue('Half a post')
})

test('a field is checked when it is left, before the form is sent', async ({ page }) => {
  await page.goto('/posts/create')
  const check = page.waitForResponse((r) => r.request().headers()['precognition'] === 'true')
  await page.getByLabel('Title').fill('HELLO, TUG')
  await page.getByLabel('Body').focus()
  expect((await check).status()).toBe(422)

  await expect(page.getByText('another post has that title')).toBeVisible()
  // The body has only been entered, not left: it isn't wrong yet.
  await expect(page.getByText('body is required')).toHaveCount(0)
})

test('a new post is made, and the page after says so', async ({ page }) => {
  await page.goto('/posts/create')
  await page.getByLabel('Title').fill('Forms, at last')
  await page.getByLabel('Body').fill('Validation and flash messages, from Go.')
  await page.getByLabel(/Tags/).fill('go, forms')
  await page.getByRole('button', { name: 'Create post' }).click()

  await expect(page.getByRole('heading', { name: 'Forms, at last' })).toBeVisible()
  await expect(page.getByRole('status')).toHaveText('Post created')
  await expect(page.getByRole('listitem').filter({ hasText: 'forms' })).toBeVisible()

  // The message is for that page alone.
  await page.getByRole('link', { name: 'tug' }).click()
  await expect(page.getByRole('heading', { name: 'Posts' })).toBeVisible()
  await expect(page.getByRole('status')).toHaveCount(0)
})

test('a post is edited in place', async ({ page }) => {
  await page.goto('/posts/2/edit')
  await expect(page.getByLabel('Title')).toHaveValue('No API in between')
  await page.getByLabel('Title').fill('No API, no reloads')
  await page.getByRole('button', { name: 'Save' }).click()

  await expect(page.getByRole('heading', { name: 'No API, no reloads' })).toBeVisible()
  await expect(page.getByRole('status')).toHaveText('Post updated')
  await expect(page).toHaveURL(/\/posts\/2$/)
})

test('deleting a post goes back to the list, without it', async ({ page }) => {
  await page.goto('/posts/3')
  await expect(page.getByRole('list')).toHaveCount(1) // the tags, none of them
  await page.getByRole('button', { name: 'Delete' }).click()

  await expect(page).toHaveURL(/\/$/)
  await expect(page.getByRole('status')).toHaveText('Post deleted')
  await expect(page.getByRole('link', { name: 'Hello, tug' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Delete me' })).toHaveCount(0)
})
