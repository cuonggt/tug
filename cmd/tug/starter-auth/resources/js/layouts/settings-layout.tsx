import { Link, usePage } from '@inertiajs/react'
import type { ReactNode } from 'react'
import Heading from '@/components/heading'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { route } from '@/tug/routes'

const pages = [
  { title: 'Profile', href: route('profile.edit') },
  { title: 'Security', href: route('security.edit') },
  { title: 'Appearance', href: route('appearance.edit') },
]

// SettingsLayout is around the settings pages, inside the app's layout:
// their names, down the side, or across the top on a phone.
export default function SettingsLayout({ children }: { children: ReactNode }) {
  const { url } = usePage()
  return (
    <>
      <Heading title="Settings" description="Your profile, your account's security, and how the app looks" />
      <div className="flex flex-col gap-6 lg:flex-row lg:gap-12">
        <aside className="lg:w-48">
          <nav aria-label="Settings" className="flex gap-1 lg:flex-col">
            {pages.map((page) => (
              <Button
                key={page.href}
                variant="ghost"
                size="sm"
                asChild
                className={cn('justify-start', url.startsWith(page.href) && 'bg-muted')}
              >
                <Link href={page.href}>{page.title}</Link>
              </Button>
            ))}
          </nav>
        </aside>
        <Separator className="lg:hidden" />
        <div className="max-w-xl flex-1 space-y-12">{children}</div>
      </div>
    </>
  )
}
