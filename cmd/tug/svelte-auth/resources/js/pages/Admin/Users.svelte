<script module lang="ts">
  // when is a time as the page shows it, the same on the server as in any
  // browser, whatever its language or zone.
  function when(at: string): string {
    return `${new Date(at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' })} UTC`
  }
</script>

<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import Search from '@lucide/svelte/icons/search'
  import Head from '@/Head.svelte'
  import { Badge } from '@/components/ui/badge'
  import { Button, buttonVariants } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // Users is the admins' page of the users, the newest first, a page at a
  // time, found by their email or name: whether each has verified their
  // email, turned two-factor logins on, is an admin, or suspended, and when
  // they joined; and the buttons that suspend one, restore them, or act as
  // them, to see the app as they do. Its handlers are in admin.go, and who
  // may do what to whom in abilities.go.
  let { list, search }: PageProps<'Admin/Users'> = $props()

  function submit(e: SubmitEvent) {
    e.preventDefault()
    const find = new FormData(e.currentTarget as HTMLFormElement).get('search')
    router.get(route('users.index'), find ? { search: String(find) } : {}, { preserveState: true })
  }
</script>

<Head title="Users" />
<div class="space-y-8">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold tracking-tight">Users</h1>
      <p class="text-muted-foreground">Everyone with an account, the newest first.</p>
    </div>
    <form role="search" onsubmit={submit} class="flex gap-2">
      <Input type="search" name="search" value={search} placeholder="Email or name" aria-label="Email or name" class="w-56" />
      <Button type="submit" variant="outline">
        <Search />
        Find
      </Button>
    </form>
  </div>
  {#if list.data.length === 0}
    <p class="text-muted-foreground">{search ? `No one's email or name has “${search}” in it.` : 'No one has an account yet.'}</p>
  {:else}
    <ul class="divide-y rounded-lg border">
      {#each list.data as user (user.id)}
        <li class="flex flex-wrap items-center gap-3 p-4">
          <div class="min-w-0 flex-1 text-sm">
            <p class="flex flex-wrap items-center gap-2 font-medium">
              <span class="truncate">{user.name}</span>
              {#if user.admin}<Badge variant="secondary">Admin</Badge>{/if}
              {#if user.suspendedAt}<Badge variant="destructive">Suspended</Badge>{/if}
              {#if !user.verified}<Badge variant="outline">Email not verified</Badge>{/if}
              {#if user.twoFactor}<Badge variant="outline">Two-factor</Badge>{/if}
            </p>
            <p class="truncate text-muted-foreground">{user.email}</p>
            <p class="text-xs text-muted-foreground">
              Joined {when(user.createdAt)}{#if user.suspendedAt}, suspended {when(user.suspendedAt)}{/if}
            </p>
          </div>
          <div class="flex gap-2">
            {#if user.can.actAs}
              <Button variant="outline" size="sm" aria-label={`Act as ${user.name}`} onclick={() => router.post(route('users.act', { id: user.id }))}>
                Act as
              </Button>
            {/if}
            {#if user.suspendedAt}
              <Button
                variant="outline"
                size="sm"
                aria-label={`Restore ${user.name}`}
                onclick={() => router.post(route('users.restore', { id: user.id }), {}, { preserveScroll: true })}
              >
                Restore
              </Button>
            {:else if user.can.suspend}
              <Button
                variant="destructive"
                size="sm"
                aria-label={`Suspend ${user.name}`}
                onclick={() => router.post(route('users.suspend', { id: user.id }), {}, { preserveScroll: true })}
              >
                Suspend
              </Button>
            {/if}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
  {#if list.prev_page_url || list.next_page_url}
    <nav aria-label="Pages" class="flex justify-between">
      {#if list.prev_page_url}
        <Link href={list.prev_page_url} class={buttonVariants({ variant: 'outline', size: 'sm' })}>Newer</Link>
      {:else}
        <span></span>
      {/if}
      {#if list.next_page_url}
        <Link href={list.next_page_url} class={buttonVariants({ variant: 'outline', size: 'sm' })}>Older</Link>
      {/if}
    </nav>
  {/if}
</div>
