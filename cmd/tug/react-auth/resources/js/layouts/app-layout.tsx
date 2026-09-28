import { Link, usePage } from '@inertiajs/react'
import type { ReactNode } from 'react'
import AppLogo from '@/components/app-logo'
import UserMenu from '@/components/user-menu'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { route } from '@/tug/routes'

// nav is the app's own pages, for users who've logged in: add each page to
// it as the app grows.
const nav = [{ title: 'Dashboard', href: route('dashboard') }]

// AppLayout is around the app's pages: its name, where to go, and who's
// logged in, or the way in for a guest, as on an error page.
export default function AppLayout({ children }: { children: ReactNode }) {
  const { props, url } = usePage()
  const user = props.auth.user
  return (
    <div className="flex min-h-svh flex-col">
      <header className="border-b">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-6 px-4">
          <Link href={user ? route('dashboard') : route('home')} className="rounded-md">
            <AppLogo />
          </Link>
          {user && (
            <nav aria-label="Main" className="flex items-center gap-1 text-sm">
              {nav.map((item) => (
                <Link
                  key={item.href}
                  href={item.href}
                  className={cn(
                    'rounded-md px-3 py-1.5 text-muted-foreground transition-colors hover:text-foreground',
                    url.startsWith(item.href) && 'bg-accent text-accent-foreground',
                  )}
                >
                  {item.title}
                </Link>
              ))}
            </nav>
          )}
          <div className="ml-auto flex items-center gap-2">
            {user ? (
              <UserMenu user={user} />
            ) : (
              <>
                <Button variant="ghost" size="sm" asChild>
                  <Link href={route('login')}>Log in</Link>
                </Button>
                <Button size="sm" asChild>
                  <Link href={route('register')}>Register</Link>
                </Button>
              </>
            )}
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8">{children}</main>
    </div>
  )
}
