/*
|--------------------------------------------------------------------------
| Routes file
|--------------------------------------------------------------------------
|
| The routes file is used for defining the HTTP routes.
|
*/

import { readFile } from 'node:fs/promises'
import app from '@adonisjs/core/services/app'
import router from '@adonisjs/core/services/router'

type Author = { id: number; name: string }

type Post = {
  id: number
  title: string
  body: string
  author: Author
  tags: string[]
  published_at: string
}

type Comment = { id: number; author: string; body: string }

type Props = { post: Post; comments: Comment[] }

/**
 * The adapter types a page's props by its component, from the frontend's
 * pages when there are some. This app has none, so the one page it renders
 * is declared here.
 */
declare module '@adonisjs/inertia/types' {
  export interface InertiaPages {
    'Posts/Show': Props
  }
}

/**
 * The props are bench/page/page.json's, read once as the app boots, as every
 * app in the benchmark reads them, by their path from the app's directory.
 * In production that's the parent of the app's root, which is then build/,
 * the compiled app the server runs.
 */
const appDir = app.inProduction ? new URL('../', app.appRoot) : app.appRoot
const page: Props = JSON.parse(await readFile(new URL('../../page/page.json', appDir), 'utf8'))

router
  .get('/posts/:id', ({ params, inertia }) => {
    return inertia.render('Posts/Show', {
      // page.json's post has its id first, so the route's id, set over it,
      // stays the first key.
      post: { ...page.post, id: params.id },
      comments: page.comments,
    })
  })
  .where('id', router.matchers.number())
