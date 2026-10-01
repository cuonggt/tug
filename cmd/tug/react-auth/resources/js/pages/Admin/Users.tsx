import { Head, Link, router } from '@inertiajs/react'
import { Search } from 'lucide-react'
import { type FormEvent, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Users is the admins' page of the users, the newest first, a page at a
// time, found by their email or name: whether each has verified their
// email, turned two-factor logins on, is an admin, or suspended, and when
// they joined; and the buttons that suspend one, restore them, or act as
// them, to see the app as they do. Its handlers are in admin.go, and who
// may do what to whom in abilities.go.
export default function Users({ list, search }: PageProps<'Admin/Users'>) {
  const [find, setFind] = useState(search)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    router.get(route('users.index'), find ? { search: find } : {}, { preserveState: true })
  }
  return (
    <>
      <Head title="Users" />
      <div className="space-y-8">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight">Users</h1>
            <p className="text-muted-foreground">Everyone with an account, the newest first.</p>
          </div>
          <form role="search" onSubmit={submit} className="flex gap-2">
            <Input
              type="search"
              value={find}
              onChange={(e) => setFind(e.target.value)}
              placeholder="Email or name"
              aria-label="Email or name"
              className="w-56"
            />
            <Button type="submit" variant="outline">
              <Search />
              Find
            </Button>
          </form>
        </div>
        {list.data.length === 0 ? (
          <p className="text-muted-foreground">{search ? `No one's email or name has “${search}” in it.` : 'No one has an account yet.'}</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {list.data.map((user) => (
              <li key={user.id} className="flex flex-wrap items-center gap-3 p-4">
                <div className="min-w-0 flex-1 text-sm">
                  <p className="flex flex-wrap items-center gap-2 font-medium">
                    <span className="truncate">{user.name}</span>
                    {user.admin && <Badge variant="secondary">Admin</Badge>}
                    {user.suspendedAt && <Badge variant="destructive">Suspended</Badge>}
                    {!user.verified && <Badge variant="outline">Email not verified</Badge>}
                    {user.twoFactor && <Badge variant="outline">Two-factor</Badge>}
                  </p>
                  <p className="truncate text-muted-foreground">{user.email}</p>
                  <p className="text-xs text-muted-foreground">
                    Joined {when(user.createdAt)}
                    {user.suspendedAt && `, suspended ${when(user.suspendedAt)}`}
                  </p>
                </div>
                <div className="flex gap-2">
                  {user.can.actAs && (
                    <Button variant="outline" size="sm" aria-label={`Act as ${user.name}`} onClick={() => router.post(route('users.act', { id: user.id }))}>
                      Act as
                    </Button>
                  )}
                  {user.suspendedAt ? (
                    <Button
                      variant="outline"
                      size="sm"
                      aria-label={`Restore ${user.name}`}
                      onClick={() => router.post(route('users.restore', { id: user.id }), {}, { preserveScroll: true })}
                    >
                      Restore
                    </Button>
                  ) : (
                    user.can.suspend && (
                      <Button
                        variant="destructive"
                        size="sm"
                        aria-label={`Suspend ${user.name}`}
                        onClick={() => router.post(route('users.suspend', { id: user.id }), {}, { preserveScroll: true })}
                      >
                        Suspend
                      </Button>
                    )
                  )}
                </div>
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
