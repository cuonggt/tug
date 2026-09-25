import { Head, Link } from '@inertiajs/react'
import type { PostsShowProps } from '../../types'

export default function Show({ appName, post }: PostsShowProps) {
  return (
    <main>
      <Head title={post.title} />
      <p className="brand">
        <Link href="/">{appName}</Link> / posts
      </p>
      <h1>{post.title}</h1>
      <p>{post.body}</p>
      <ul className="tags">
        {post.tags.map((tag) => (
          <li key={tag}>{tag}</li>
        ))}
      </ul>
      <Link href={`/posts/${post.id}`} method="delete" as="button" className="danger">
        Delete
      </Link>
    </main>
  )
}
