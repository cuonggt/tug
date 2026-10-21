<script lang="ts">
  import { Link, router } from '@inertiajs/svelte'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { useLang } from '@/lib/lang'
  import { listen } from '@/lib/broadcasts'
  import { cn } from '@/lib/utils'
  import type { PageProps } from '@/tug/pages'

  // The page says its own words in the request's language.
  const { t, d } = useLang()

  // Notifications is what happened to the user's account, the newest first,
  // a page at a time, which its handler, in notifications.go, marks read as
  // it shows them: the ones new to this page have a dot. A notification made
  // while it's open shows at once.
  let { list }: PageProps<'Notifications'> = $props()

  onMount(() => listen('notification', () => router.reload({ only: ['list', 'bell'] })))

  // when is a time as the page shows it, with the name of the zone it's in:
  // the app's own, and the reader's once the page is in their browser.
  const when = (at: string) =>
    d(at, { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' })
</script>

<Head title={t('Notifications')} />
<div class="space-y-8">
  <div class="space-y-1">
    <h1 class="text-2xl font-semibold tracking-tight">{t('Notifications')}</h1>
    <p class="text-muted-foreground">{t('What happened to your account, the newest first.')}</p>
  </div>
  {#if list.data.length === 0}
    <p class="text-muted-foreground">{t('Nothing has happened yet.')}</p>
  {:else}
    <ul class="divide-y rounded-lg border">
      {#each list.data as n (n.id)}
        <li>
          <Link href={n.link} class="flex items-start gap-3 p-4 transition-colors hover:bg-accent/50">
            <span class={cn('mt-1.5 size-2 shrink-0 rounded-full', n.new && 'bg-primary')}></span>
            <span class="min-w-0 flex-1">
              <span class={cn('block text-sm', n.new && 'font-medium')}>
                {#if n.new}<span class="sr-only">{t('New:')} </span>{/if}{n.line}
              </span>
              <span class="block text-xs text-muted-foreground">{when(n.createdAt)}</span>
            </span>
          </Link>
        </li>
      {/each}
    </ul>
  {/if}
  {#if list.prev_page_url || list.next_page_url}
    <nav aria-label={t('Pages')} class="flex justify-between">
      {#if list.prev_page_url}
        <Link href={list.prev_page_url} class={buttonVariants({ variant: 'outline', size: 'sm' })}>{t('Newer')}</Link>
      {:else}
        <span></span>
      {/if}
      {#if list.next_page_url}
        <Link href={list.next_page_url} class={buttonVariants({ variant: 'outline', size: 'sm' })}>{t('Older')}</Link>
      {/if}
    </nav>
  {/if}
</div>
