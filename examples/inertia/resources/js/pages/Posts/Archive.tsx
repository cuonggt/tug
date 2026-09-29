import { Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import Pager from '../../Pager'
import type { PageProps } from '../../tug/pages'
import { route } from '../../tug/routes'

export default function Archive({ posts }: PageProps<'Posts/Archive'>) {
  return (
    <Layout>
      <Head title="Every post" />
      <h1>Every post</h1>
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
      <Pager page={posts} />
      {/* A plain link: Inertia's <Link> visits a page, where this is a file to save. */}
      <p>
        <a href={route('posts.export')}>Download as CSV</a>
      </p>
    </Layout>
  )
}
