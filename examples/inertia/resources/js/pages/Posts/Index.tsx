import { Deferred, Head, InfiniteScroll, Link, router } from '@inertiajs/react'
import Layout from '../../Layout'
import type { PostsIndexProps, Stats } from '../../types'

export default function Index({ posts, stats }: PostsIndexProps) {
  return (
    <Layout>
      <Head title="Posts" />
      <h1>Posts</h1>
      <Deferred data="stats" fallback={<p className="stats">Counting…</p>}>
        {stats && <StatsLine stats={stats} />}
      </Deferred>
      <p>
        <Link href="/posts/create" className="button">
          New post
        </Link>
      </p>
      {/* Each click asks the server for the next page, which Go's
          inertia.Scroll serves and the client adds to the list. */}
      <InfiniteScroll
        data="posts"
        manual
        next={({ fetch, loadingNext, hasNext }) =>
          hasNext && (
            <button type="button" onClick={fetch} disabled={loadingNext}>
              {loadingNext ? 'Loading…' : 'More posts'}
            </button>
          )
        }
      >
        <ul className="posts">
          {posts.data.map((post) => (
            <li key={post.id}>
              <Link href={`/posts/${post.id}`}>{post.title}</Link>
            </li>
          ))}
        </ul>
      </InfiniteScroll>
    </Layout>
  )
}

function StatsLine({ stats }: { stats: Stats }) {
  return (
    <p className="stats" data-testid="stats">
      {stats.posts} posts, {stats.words} words{' '}
      <button type="button" onClick={() => router.reload({ only: ['stats'] })}>
        Recount
      </button>
    </p>
  )
}
