import { Head } from '@inertiajs/react'
import Layout from '../Layout'
import type { PageProps } from '../tug/pages'

// Dashboard is for users who've logged in: its route in main.go is
// wrapped in usersOnly, which sends a guest to log in first.
export default function Dashboard({ user }: PageProps<'Dashboard'>) {
  const since = new Date(user.createdAt).toLocaleDateString(undefined, { dateStyle: 'long' })
  return (
    <Layout>
      <Head title="Dashboard" />
      <h1>Dashboard</h1>
      <p>
        Hello, {user.name}. You've had an account since {since}.
      </p>
      <p className="hint">
        This page is <code>resources/js/pages/Dashboard.tsx</code>, and its handler is <code>dashboard</code> in{' '}
        <code>main.go</code>, which is handed the user.
      </p>
    </Layout>
  )
}
