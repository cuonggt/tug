import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import PostForm from '../../PostForm'
import type { PostsEditProps } from '../../types'

export default function Edit({ post }: PostsEditProps) {
  return (
    <Layout>
      <Head title={`Edit ${post.title}`} />
      <h1>Edit post</h1>
      <PostForm action={`/posts/${post.id}`} method="put" post={post} submit="Save" />
    </Layout>
  )
}
