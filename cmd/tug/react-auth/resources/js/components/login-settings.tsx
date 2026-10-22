import { router } from '@inertiajs/react'
import { LogOut, Monitor } from 'lucide-react'
import { useEffect } from 'react'
import Heading from '@/components/heading'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { useLang } from '@/lib/lang'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// LoginSettings lists the browsers the user is logged in from, this one
// first, and logs one of the others out, or all of them: a browser left
// logged in on a lost laptop, say. The handlers are in logins.go.
export default function LoginSettings({ browsers }: PageProps<'Settings/Security'>) {
  const { t, d } = useLang()
  // day is a date as the page shows it, in the page's own language, and in
  // the reader's own zone once the page is in their browser.
  const day = (at: string) => d(at, { dateStyle: 'medium' })

  // A browser of the account opening or closing shows at once: each page
  // is on the user's channel while it's open, and the app says as one
  // comes or goes.
  useEffect(() => {
    const reload = () => router.reload({ only: ['browsers'] })
    const came = listen('presence.joined', reload)
    const went = listen('presence.left', reload)
    return () => {
      came()
      went()
    }
  }, [])

  return (
    <section className="space-y-6">
      <Heading small title={t('Browsers')} description={t("Where you're logged in. Log out of one you don't know, or of every one but this.")} />
      {browsers.length > 0 && (
        <ul className="divide-y rounded-lg border">
          {browsers.map((browser) => (
            <li key={browser.id} className="flex items-center gap-3 p-3">
              <Monitor className="size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1 text-sm">
                <p className="flex items-center gap-2 font-medium">
                  <span className="truncate">{capital(browser.name)}</span>
                  {browser.current && <Badge variant="secondary">{t('This browser')}</Badge>}
                </p>
                <p className="text-muted-foreground">
                  {browser.ip}, {t('logged in :date', { date: day(browser.began) })},{' '}
                  {browser.open ? t('open now') : t('last seen :date', { date: day(browser.lastSeen) })}
                </p>
              </div>
              {!browser.current && (
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('Log out of :name', { name: browser.name })}
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
          {t('Log out of every other browser')}
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
