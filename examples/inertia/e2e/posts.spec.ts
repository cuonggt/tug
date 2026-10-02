import { readFile } from 'node:fs/promises'
import { expect, test } from '@playwright/test'

// The tests share one server and run in order: the first ones need the 25
// posts the example starts with, and the later ones add, change and delete
// posts.

test('the list shows first, and its stats once counted', async ({ page, request }) => {
  // The first visit's page has the posts, and names the stats for later.
  const html = await (await request.get('/')).text()
  const data = html.match(/<script data-page="app" type="application\/json">(.*?)<\/script>/)
  const first = JSON.parse(data![1])
  expect(first.props.posts.data).toHaveLength(10)
  expect(first.scrollProps.posts).toMatchObject({ currentPage: 1, nextPage: 2 })
  expect(first.props.stats).toBeUndefined()
  expect(first.deferredProps).toEqual({ default: ['stats'] })

  await page.goto('/')
  await expect(page.getByRole('link', { name: 'Hello, tug' })).toBeVisible()
  await expect(page.getByTestId('stats')).toContainText('25 posts, 160 words')
})

test('a link changes the page without loading a new one', async ({ page }) => {
  await page.goto('/')
  await page.evaluate(() => Object.assign(window, { stillHere: true }))
  await page.getByRole('link', { name: 'Hello, tug' }).click()

  await expect(page.getByRole('heading', { name: 'Hello, tug' })).toBeVisible()
  await expect(page).toHaveURL(/\/posts\/1$/)
  await expect(page).toHaveTitle('Hello, tug · tug')
  // The head the post's handler gave it, which the client keeps in the
  // document's head.
  const description = page.locator('meta[name="description"]')
  await expect(description).toHaveAttribute('content', 'Pages rendered by React, with props from Go handlers.')
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

test('the list gets its next page as asked, until there are no more', async ({ page }) => {
  await page.goto('/')
  const posts = page.locator('.posts li')
  await expect(posts).toHaveCount(10)

  const next = page.waitForResponse((r) => r.request().headers()['x-inertia-infinite-scroll-merge-intent'] === 'append')
  await page.getByRole('button', { name: 'More posts' }).click()
  const body = await (await next).json()
  expect(body.props.posts.data).toHaveLength(10)
  await expect(posts).toHaveCount(20)
  await expect(posts.first()).toHaveText('Hello, tug') // added to, not replaced

  await page.getByRole('button', { name: 'More posts' }).click()
  await expect(posts).toHaveCount(25)
  await expect(page.getByRole('button', { name: 'More posts' })).toHaveCount(0)
})

test('every post comes in numbered pages, with a pager', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('link', { name: 'Every post, by page' }).click()
  await expect(page).toHaveURL(/\/posts$/)
  await expect(page.getByText('1 to 10 of 25')).toBeVisible()
  await expect(page.locator('.posts li').first()).toHaveText('Hello, tug')

  const pager = page.getByRole('navigation', { name: 'Pages' })
  await pager.getByRole('link', { name: '3', exact: true }).click()
  await expect(page).toHaveURL(/\/posts\?page=3$/)
  await expect(page.getByText('21 to 25 of 25')).toBeVisible()
  await expect(pager.getByRole('link', { name: '3', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(pager.getByRole('link', { name: 'Next' })).toHaveCount(0)

  await pager.getByRole('link', { name: 'Previous' }).click()
  await expect(page).toHaveURL(/\/posts\?page=2$/)
  await expect(page.locator('.posts li').first()).toHaveText('Post 11')
})

test('every post downloads as CSV, a file to save', async ({ page }) => {
  await page.goto('/posts')
  const download = page.waitForEvent('download')
  await page.getByRole('link', { name: 'Download as CSV' }).click()
  const file = await download
  expect(file.suggestedFilename()).toBe('posts.csv')
  const lines = (await readFile(await file.path(), 'utf8')).trimEnd().split('\n')
  expect(lines[0]).toBe('id,title,tags')
  expect(lines[1]).toBe('1,"Hello, tug","go, inertia"')
  expect(lines).toHaveLength(26)
  // The page is still the one the link was on.
  await expect(page).toHaveURL(/\/posts$/)
})

test('a page that is not there is shown as the error page, with its status', async ({ page }) => {
  const response = await page.goto('/posts/999')
  expect(response!.status()).toBe(404)
  await expect(page.getByRole('heading', { name: 'Not found' })).toBeVisible()
  await expect(page.getByText('404: post not found')).toBeVisible()

  // And from a link, by Inertia's client, the same.
  await page.getByRole('link', { name: 'Back to the posts' }).click()
  await expect(page.getByRole('heading', { name: 'Posts' })).toBeVisible()
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

test('a post made in another browser shows in this one, on its page and in the stats', async ({ page, context, browser }) => {
  const archiveListens = page.waitForResponse((r) => r.url().endsWith('/posts/events'))
  await page.goto('/posts?page=3')
  await archiveListens
  const index = await context.newPage()
  const indexListens = index.waitForResponse((r) => r.url().endsWith('/posts/events'))
  await index.goto('/')
  await indexListens
  const counted = Number((await index.getByTestId('stats').innerText()).match(/^(\d+) posts/)![1])

  const elsewhere = await browser.newPage()
  await elsewhere.goto('/posts/create')
  await elsewhere.getByLabel('Title').fill('Made elsewhere')
  await elsewhere.getByLabel('Body').fill('Shown in the other browser as it is made.')
  await elsewhere.getByRole('button', { name: 'Create post' }).click()
  await expect(elsewhere.getByRole('heading', { name: 'Made elsewhere' })).toBeVisible()
  await elsewhere.close()

  // The last page of every post, where the new one goes, and the list's
  // stats, each without a visit.
  await expect(page.locator('.posts li').last()).toHaveText('Made elsewhere')
  await expect(page.getByText(`of ${counted + 1}`)).toBeVisible()
  await expect(index.getByTestId('stats')).toContainText(`${counted + 1} posts`)
})
