import { Head, Link } from '@inertiajs/react'
import Layout from '../Layout'
import type { PageProps } from '../tug/pages'
import { route } from '../tug/routes'

const titles: Record<number, string> = {
  403: 'Not allowed',
  404: 'Not found',
  500: 'Something went wrong',
}

// Error is the page tug shows errors with (Config.ErrorPage in main.go),
// with the response's own status.
export default function Error({ status, message }: PageProps<'Error'>) {
  const title = titles[status] ?? 'Something went wrong'
  return (
    <Layout>
      <Head title={title} />
      <h1>{title}</h1>
      <p>
        {status}: {message}
      </p>
      <p>
        <Link href={route('home')}>Back home</Link>
      </p>
    </Layout>
  )
}
