import { Head, Link } from '@inertiajs/react'
import Layout from '../Layout'
import type { ErrorProps } from '../types'

const titles: Record<number, string> = {
  403: 'Not allowed',
  404: 'Not found',
  500: 'Something went wrong',
}

// Error is the page tug's error handler shows errors with (Config.ErrorPage),
// with the response's own status.
export default function Error({ status, message }: ErrorProps) {
  const title = titles[status] ?? 'Something went wrong'
  return (
    <Layout>
      <Head title={title} />
      <h1>{title}</h1>
      <p>
        {status}: {message}
      </p>
      <p>
        <Link href="/">Back to the posts</Link>
      </p>
    </Layout>
  )
}
