<script lang="ts">
  import { Link, page, router } from '@inertiajs/svelte'
  import Bell from '@lucide/svelte/icons/bell'
  import type { Snippet } from 'svelte'
  import AppLogo from '@/components/AppLogo.svelte'
  import UserMenu from '@/components/UserMenu.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { listen } from '@/lib/broadcasts'
  import { cn } from '@/lib/utils'
  import { route } from '@/tug/routes'

  // nav is the app's own pages, for users who've logged in: add each page to
  // it as the app grows. An admin has theirs too, as the shared can says.
  const nav = [{ title: 'Dashboard', href: route('dashboard') }]
  const adminNav = [{ title: 'Failed jobs', href: route('failed-jobs.index') }]

  // AppLayout is around the app's pages: its name, where to go, and who's
  // logged in, or the way in for a guest, as on an error page. A link that
  // looks like a button is Inertia's Link with the button's classes, as
  // shadcn-svelte's Button makes a plain <a>, which loads the whole page.
  let { children }: { children: Snippet } = $props()
  let user = $derived(page.props.auth.user)
  let items = $derived([...nav, ...(page.props.can.seeFailedJobs ? adminNav : [])])
  let unread = $derived(page.props.bell.unread)
  let userID = $derived(user?.id)

  // A notification made in another tab, or on another device, is counted
  // at once: the app says so on the user's own channel. By the user's ID,
  // so a page's new props don't start it again.
  $effect(() => {
    if (!userID) return
    return listen('notification', () => router.reload({ only: ['bell'] }))
  })
</script>

<div class="flex min-h-svh flex-col">
  <header class="border-b">
    <div class="mx-auto flex h-14 w-full max-w-6xl items-center gap-6 px-4">
      <Link href={user ? route('dashboard') : route('home')} class="rounded-md">
        <AppLogo />
      </Link>
      {#if user}
        <nav aria-label="Main" class="flex items-center gap-1 text-sm">
          {#each items as item (item.href)}
            <Link
              href={item.href}
              class={cn(
                'rounded-md px-3 py-1.5 text-muted-foreground transition-colors hover:text-foreground',
                page.url.startsWith(item.href) && 'bg-accent text-accent-foreground',
              )}
            >
              {item.title}
            </Link>
          {/each}
        </nav>
      {/if}
      <div class="ml-auto flex items-center gap-2">
        {#if user}
          <Link
            href={route('notifications.index')}
            class={cn(buttonVariants({ variant: 'ghost', size: 'icon' }), 'relative')}
            aria-label={unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'}
          >
            <Bell />
            {#if unread > 0}
              <span
                class="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground"
              >
                {unread > 99 ? '99+' : unread}
              </span>
            {/if}
          </Link>
          <UserMenu {user} />
        {:else}
          <Link href={route('login')} class={buttonVariants({ variant: 'ghost', size: 'sm' })}>Log in</Link>
          <Link href={route('register')} class={buttonVariants({ size: 'sm' })}>Register</Link>
        {/if}
      </div>
    </div>
  </header>
  <main class="mx-auto w-full max-w-6xl flex-1 px-4 py-8">{@render children()}</main>
</div>
