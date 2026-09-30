import { Head, Link, router } from '@inertiajs/react'
import { useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { cn } from '@/lib/utils'
import type { PageProps } from '@/tug/pages'

// Notifications is what happened to the user's account, the newest first,
// a page at a time, which its handler, in notifications.go, marks read as
// it shows them: the ones new to this page have a dot. A notification made
// while it's open shows at once.
export default function Notifications({ list }: PageProps<'Notifications'>) {
  useEffect(() => listen('notification', () => router.reload({ only: ['list', 'bell'] })), [])
  return (
    <>
      <Head title="Notifications" />
      <div className="space-y-8">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">Notifications</h1>
          <p className="text-muted-foreground">What happened to your account, the newest first.</p>
        </div>
        {list.data.length === 0 ? (
          <p className="text-muted-foreground">Nothing has happened yet.</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {list.data.map((n) => (
              <li key={n.id}>
                <Link href={n.link} className="flex items-start gap-3 p-4 transition-colors hover:bg-accent/50">
                  <span className={cn('mt-1.5 size-2 shrink-0 rounded-full', n.new && 'bg-primary')} />
                  <span className="min-w-0 flex-1">
                    <span className={cn('block text-sm', n.new && 'font-medium')}>
                      {n.new && <span className="sr-only">New: </span>}
                      {n.line}
                    </span>
                    <span className="block text-xs text-muted-foreground">{when(n.createdAt)}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        {(list.prev_page_url || list.next_page_url) && (
          <nav aria-label="Pages" className="flex justify-between">
            {list.prev_page_url ? (
              <Button variant="outline" size="sm" asChild>
                <Link href={list.prev_page_url}>Newer</Link>
              </Button>
            ) : (
              <span />
            )}
            {list.next_page_url && (
              <Button variant="outline" size="sm" asChild>
                <Link href={list.next_page_url}>Older</Link>
              </Button>
            )}
          </nav>
        )}
      </div>
    </>
  )
}

// when is a time as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function when(at: string): string {
  return `${new Date(at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' })} UTC`
}
