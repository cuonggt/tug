import { Head, Link } from '@inertiajs/react'
import { Database, KeyRound, LayoutDashboard, Mail, Package, ShieldCheck } from 'lucide-react'
import AppLogo from '@/components/app-logo'
import { Button } from '@/components/ui/button'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// what is what the app comes with, and where it is, until this page says
// what the app is for.
const what = [
  { icon: KeyRound, title: 'Accounts', text: 'Registering, logging in, and a forgotten password reset by mail.', where: 'auth.go' },
  { icon: Mail, title: 'Verified email', text: 'A link mailed to each new email, which the dashboard waits for.', where: 'verify.go' },
  { icon: ShieldCheck, title: 'Two-factor logins', text: 'Codes from an authenticator app, and recovery codes.', where: 'twofactor.go' },
  { icon: LayoutDashboard, title: 'Settings', text: 'Profile, email, password, appearance, and deleting the account.', where: 'settings.go' },
  { icon: Database, title: 'SQLite', text: 'Users in database/sql, and migrations that run at start.', where: 'users.go' },
  { icon: Package, title: 'One binary', text: 'tug build puts the frontend inside the Go server.', where: 'Dockerfile' },
]

// Home is the landing page, main.go's home, with no layout around it.
export default function Home({ appName, auth }: PageProps<'Home'>) {
  return (
    <>
      <Head title="Welcome" />
      <div className="flex min-h-svh flex-col">
        <header className="mx-auto flex h-16 w-full max-w-5xl items-center justify-between px-4">
          <AppLogo />
          <nav className="flex items-center gap-2">
            {auth.user ? (
              <Button size="sm" asChild>
                <Link href={route('dashboard')}>Dashboard</Link>
              </Button>
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
          </nav>
        </header>

        <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col justify-center gap-16 px-4 py-16">
          <div className="max-w-2xl space-y-6">
            <h1 className="text-4xl font-semibold tracking-tight text-balance sm:text-5xl">{appName}</h1>
            <p className="text-lg text-muted-foreground">
              Go handlers render React pages, with Inertia in between and no API to write. People make accounts, verify
              their email, and log in with a second factor if they like.
            </p>
            <div className="flex gap-3">
              <Button size="lg" asChild>
                <Link href={auth.user ? route('dashboard') : route('register')}>{auth.user ? 'Your dashboard' : 'Get started'}</Link>
              </Button>
            </div>
          </div>

          <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {what.map(({ icon: Icon, title, text, where }) => (
              <li key={title} className="rounded-xl border p-5">
                <Icon className="size-5 text-muted-foreground" />
                <h2 className="mt-3 font-medium">{title}</h2>
                <p className="mt-1 text-sm text-muted-foreground">{text}</p>
                <code className="mt-3 inline-block rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{where}</code>
              </li>
            ))}
          </ul>
        </main>

        <footer className="mx-auto w-full max-w-5xl px-4 py-8 text-sm text-muted-foreground">
          This page is <code className="font-mono">resources/js/pages/Home.tsx</code>: make it say what {appName} is for.
        </footer>
      </div>
    </>
  )
}
