import { Form, Head, Link, router } from '@inertiajs/react'
import Layout from '../../Layout'
import Pager from '../../Pager'
import type { PageProps } from '../../tug/pages'
import { form, route, type Inputs } from '../../tug/routes'

// Archive is every post, in numbered pages, or those a search found. The
// search goes by QUERY, its term in the body, and the page it renders is
// /posts still, whose pager asks for a page of what it found by its body
// too, as a reload of a page a QUERY rendered sends the same body again.
// The posts are live: a post made, changed or deleted in another browser
// shows on the page in view.
export default function Archive({ term, posts }: PageProps<'Posts/Archive'>) {
  return (
    <Layout>
      <Head title={term ? `Posts with ${term}` : 'Every post'} />
      <h1>{term ? `Posts with “${term}” in their title` : 'Every post'}</h1>
      <Form<Inputs['posts.search']> action={form('posts.search')} className="search">
        {({ processing }) => (
          <>
            <input name="term" defaultValue={term} aria-label="Search titles" placeholder="Search titles" />
            <button type="submit" disabled={processing}>
              Search
            </button>
            {term && <Link href={route('posts.archive')}>Every post</Link>}
          </>
        )}
      </Form>
      <p className="stats">
        {posts.from === null ? 'No posts on this page' : `${posts.from} to ${posts.to} of ${posts.total}`}
      </p>
      <ol className="posts" start={posts.from ?? 1}>
        {posts.data.map((post) => (
          <li key={post.id}>
            <Link href={route('posts.show', { id: post.id })}>{post.title}</Link>
          </li>
        ))}
      </ol>
      <Pager page={posts} go={term ? (page) => router.reload({ data: { page }, only: ['posts'] }) : undefined} />
      {/* A plain link: Inertia's <Link> visits a page, where this is a file to save. */}
      <p>
        <a href={route('posts.export')}>Download as CSV</a>
      </p>
    </Layout>
  )
}
