import { Link } from '@inertiajs/react'
import type { ReactNode } from 'react'
import { AppLogoIcon } from '@/components/app-logo'
import { route } from '@/tug/routes'

// AuthLayout is around logging in, registering and the rest of Auth/: the
// app's mark, the page's title and what to do, and a card for the form.
// A page names them with a static layout, as
// Login.layout = { title: 'Log in', description: '...' }.
export default function AuthLayout({ title, description, children }: { title?: string; description?: string; children: ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center bg-muted/40 p-6 md:p-10">
      <div className="flex w-full max-w-sm flex-col gap-6">
        <Link href={route('home')} aria-label="Home" className="self-center rounded-md">
          <AppLogoIcon className="size-10 text-base" />
        </Link>
        <div className="rounded-xl border bg-card p-6 text-card-foreground shadow-xs sm:p-8">
          <div className="mb-6 space-y-1.5 text-center">
            <h1 className="text-xl font-semibold tracking-tight">{title}</h1>
            {description && <p className="text-sm text-balance text-muted-foreground">{description}</p>}
          </div>
          {children}
        </div>
      </div>
    </div>
  )
}
