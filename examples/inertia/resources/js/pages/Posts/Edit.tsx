import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import PostForm from '../../PostForm'
import type { PageProps } from '../../tug/pages'
import { route } from '../../tug/routes'

export default function Edit({ post }: PageProps<'Posts/Edit'>) {
  return (
    <Layout>
      <Head title={`Edit ${post.title}`} />
      <h1>Edit post</h1>
      <PostForm action={route('posts.update', { id: post.id })} method="put" post={post} submit="Save" />
    </Layout>
  )
}
