import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import PostForm from '../../PostForm'
import { form } from '../../tug/routes'

export default function Create() {
  return (
    <Layout>
      <Head title="New post" />
      <h1>New post</h1>
      <PostForm action={form('posts.store')} submit="Create post" />
    </Layout>
  )
}
