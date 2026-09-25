import { Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import type { PageProps } from '../../tug/pages'
import { route } from '../../tug/routes'

export default function Show({ post }: PageProps<'Posts/Show'>) {
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
        <Link href={route('posts.edit', { id: post.id })} className="button">
          Edit
        </Link>
        <Link href={route('posts.destroy', { id: post.id })} method="delete" as="button" className="danger">
          Delete
        </Link>
      </p>
    </Layout>
  )
}
