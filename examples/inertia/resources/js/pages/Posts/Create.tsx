import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import PostForm from '../../PostForm'

export default function Create() {
  return (
    <Layout>
      <Head title="New post" />
      <h1>New post</h1>
      <PostForm action="/posts" method="post" submit="Create post" />
    </Layout>
  )
}
