import { Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import type { PostsShowProps } from '../../types'

export default function Show({ post }: PostsShowProps) {
  return (
    <Layout>
      <Head title={post.title} />
      <h1>{post.title}</h1>
      <p>{post.body}</p>
      <ul className="tags">
        {post.tags.map((tag) => (
          <li key={tag}>{tag}</li>
        ))}
      </ul>
      <p className="actions">
        <Link href={`/posts/${post.id}/edit`} className="button">
          Edit
        </Link>
        <Link href={`/posts/${post.id}`} method="delete" as="button" className="danger">
          Delete
        </Link>
      </p>
    </Layout>
  )
}
