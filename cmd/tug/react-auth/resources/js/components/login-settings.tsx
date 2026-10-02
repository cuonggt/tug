import { router } from '@inertiajs/react'
import { LogOut, Monitor } from 'lucide-react'
import Heading from '@/components/heading'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// LoginSettings lists the browsers the user is logged in from, this one
// first, and logs one of the others out, or all of them: a browser left
// logged in on a lost laptop, say. The handlers are in logins.go.
export default function LoginSettings({ browsers }: PageProps<'Settings/Security'>) {
  return (
    <section className="space-y-6">
      <Heading small title="Browsers" description="Where you're logged in. Log out of one you don't know, or of every one but this." />
      {browsers.length > 0 && (
        <ul className="divide-y rounded-lg border">
          {browsers.map((browser) => (
            <li key={browser.id} className="flex items-center gap-3 p-3">
              <Monitor className="size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1 text-sm">
                <p className="flex items-center gap-2 font-medium">
                  <span className="truncate">{capital(browser.name)}</span>
                  {browser.current && <Badge variant="secondary">This browser</Badge>}
                </p>
                <p className="text-muted-foreground">
                  {browser.ip}, logged in {day(browser.began)}, last seen {day(browser.lastSeen)}
                </p>
              </div>
              {!browser.current && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Log out of ${browser.name}`}
                  onClick={() => router.delete(route('logins.destroy', { id: browser.id }), { preserveScroll: true })}
                >
                  <LogOut />
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      {browsers.length > 1 && (
        <Button variant="outline" onClick={() => router.delete(route('logins.destroy-others'), { preserveScroll: true })}>
          <LogOut />
          Log out of every other browser
        </Button>
      )}
    </section>
  )
}

// capital is a browser's name as a line starts with it: "a browser on
// Linux", one the app doesn't know, as "A browser on Linux".
function capital(name: string): string {
  return name.charAt(0).toUpperCase() + name.slice(1)
}

// day is a date as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function day(at: string): string {
  return new Date(at).toLocaleDateString('en-US', { dateStyle: 'medium', timeZone: 'UTC' })
}
