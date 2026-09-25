import { Link, usePage } from '@inertiajs/react'
import type { ReactNode } from 'react'
import { route } from './tug/routes'

// Layout is what every page has around it: the app's name, who's logged
// in, and the flash message the page after a form arrives with.
export default function Layout({ children }: { children: ReactNode }) {
  const { props, flash } = usePage()
  const user = props.auth.user
  return (
    <main>
      <header className="bar">
        <Link href={route('home')} className="brand">
          {props.appName}
        </Link>
        <nav>
          {user ? (
            <>
              <Link href={route('dashboard')}>{user.name}</Link>
              <Link href={route('logout')} method="post" as="button" className="link">
                Log out
              </Link>
            </>
          ) : (
            <>
              <Link href={route('login')}>Log in</Link>
              <Link href={route('register')}>Register</Link>
            </>
          )}
        </nav>
      </header>
      {flash.success && (
        <p role="status" className="flash">
          {flash.success}
        </p>
      )}
      {children}
    </main>
  )
}
