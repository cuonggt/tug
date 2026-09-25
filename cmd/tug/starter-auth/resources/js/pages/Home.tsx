import { Head, Link } from '@inertiajs/react'
import Layout from '../Layout'
import type { PageProps } from '../tug/pages'
import { route } from '../tug/routes'

// Home is the page main.go's home handler renders. Every page has
// auth.user, the user who's logged in or null: auth.go shares it.
export default function Home({ appName, auth }: PageProps<'Home'>) {
  return (
    <Layout>
      <Head title="Home" />
      <h1>{appName}</h1>
      {auth.user ? (
        <p>
          You're logged in as {auth.user.name}. <Link href={route('dashboard')}>Your dashboard</Link> is only for
          you.
        </p>
      ) : (
        <p>
          <Link href={route('register')}>Register</Link> for an account, or <Link href={route('login')}>log in</Link>{' '}
          to yours.
        </p>
      )}
      <p className="hint">
        Registering, logging in and resetting a password are in <code>auth.go</code>, and the users are in SQLite,
        in <code>app.db</code>: see <code>users.go</code>.
      </p>
    </Layout>
  )
}
