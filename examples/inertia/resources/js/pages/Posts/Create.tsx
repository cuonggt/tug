import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import PostForm from '../../PostForm'
import { route } from '../../tug/routes'

export default function Create() {
  return (
    <Layout>
      <Head title="New post" />
      <h1>New post</h1>
      <PostForm action={route('posts.store')} method="post" submit="Create post" />
    </Layout>
  )
}
