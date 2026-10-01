import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import PostForm from '../../PostForm'
import type { PageProps } from '../../tug/pages'
import { form } from '../../tug/routes'

export default function Edit({ post }: PageProps<'Posts/Edit'>) {
  return (
    <Layout>
      <Head title={`Edit ${post.title}`} />
      <h1>Edit post</h1>
      <PostForm action={form('posts.update', { id: post.id })} post={post} submit="Save" />
    </Layout>
  )
}
