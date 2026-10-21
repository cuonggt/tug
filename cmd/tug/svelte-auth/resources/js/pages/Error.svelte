<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { useLang } from '@/lib/lang'
  import { cn } from '@/lib/utils'
  import { text } from '@/tug/lang'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

  const titles: Record<number, string> = {
    403: text('Not allowed'),
    404: text('Not found'),
    500: text('Something went wrong'),
    503: text('Back in a moment'),
  }

  // Error is the page tug shows errors with (Config.ErrorPage in main.go),
  // with the response's own status, inside the app's layout.
  let { status, message }: PageProps<'Error'> = $props()
  let title = $derived(t(titles[status] ?? text('Something went wrong')))
</script>

<Head {title} />
<div class="flex flex-col items-center py-24 text-center">
  <p class="font-mono text-sm text-muted-foreground">{status}</p>
  <h1 class="mt-2 text-3xl font-semibold tracking-tight">{title}</h1>
  {#if message.toLowerCase() !== title.toLowerCase()}
    <p class="mt-4 max-w-md text-muted-foreground">{message}</p>
  {/if}
  <Link href={route('home')} class={cn(buttonVariants(), 'mt-8')}>{t('Back home')}</Link>
</div>
