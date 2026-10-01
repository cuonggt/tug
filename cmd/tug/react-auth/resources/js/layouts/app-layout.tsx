import { Link, router, usePage } from '@inertiajs/react'
import { Bell } from 'lucide-react'
import { type ReactNode, useEffect } from 'react'
import ActingBanner from '@/components/acting-banner'
import AppLogo from '@/components/app-logo'
import UserMenu from '@/components/user-menu'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { cn } from '@/lib/utils'
import { route } from '@/tug/routes'

// nav is the app's own pages, for users who've logged in: add each page to
// it as the app grows. An admin has theirs too, each as the shared can
// says they may.
const nav = [{ title: 'Dashboard', href: route('dashboard') }]
const adminNav = [
  { title: 'Users', href: route('users.index'), may: 'seeUsers' },
  { title: 'Failed jobs', href: route('failed-jobs.index'), may: 'seeFailedJobs' },
] as const

// AppLayout is around the app's pages: its name, where to go, and who's
// logged in, or the way in for a guest, as on an error page, under the
// line that says an admin is acting as the user.
export default function AppLayout({ children }: { children: ReactNode }) {
  const { props, url } = usePage()
  const user = props.auth.user
  const unread = props.bell.unread
  // A notification made in another tab, or on another device, is counted
  // at once: the app says so on the user's own channel.
  useEffect(() => {
    if (!user) return
    return listen('notification', () => router.reload({ only: ['bell'] }))
  }, [user?.id])
  return (
    <div className="flex min-h-svh flex-col">
      <ActingBanner />
      <header className="border-b">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-6 px-4">
          <Link href={user ? route('dashboard') : route('home')} className="rounded-md">
            <AppLogo />
          </Link>
          {user && (
            <nav aria-label="Main" className="flex items-center gap-1 text-sm">
              {[...nav, ...adminNav.filter((item) => props.can[item.may])].map((item) => (
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
              <>
                <Button variant="ghost" size="icon" className="relative" asChild>
                  <Link
                    href={route('notifications.index')}
                    aria-label={unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'}
                  >
                    <Bell />
                    {unread > 0 && (
                      <span className="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground">
                        {unread > 99 ? '99+' : unread}
                      </span>
                    )}
                  </Link>
                </Button>
                <UserMenu user={user} />
              </>
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
