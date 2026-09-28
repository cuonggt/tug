import { Link, usePage } from '@inertiajs/react'
import type { ReactNode } from 'react'

// Layout is what every page has around it: the app's name, and the flash
// message the page after a form arrives with.
export default function Layout({ children }: { children: ReactNode }) {
  const { props, flash } = usePage()
  return (
    <main>
      <p className="brand">
        <Link href="/">{props.appName}</Link>
      </p>
      {flash.success && (
        <p role="status" className="flash">
          {flash.success}
        </p>
      )}
      {children}
    </main>
  )
}
