<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { cn } from '@/lib/utils'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  const titles: Record<number, string> = {
    403: 'Not allowed',
    404: 'Not found',
    500: 'Something went wrong',
    503: 'Back in a moment',
  }

  // Error is the page tug shows errors with (Config.ErrorPage in main.go),
  // with the response's own status, inside the app's layout.
  let { status, message }: PageProps<'Error'> = $props()
  let title = $derived(titles[status] ?? 'Something went wrong')
</script>

<Head {title} />
<div class="flex flex-col items-center py-24 text-center">
  <p class="font-mono text-sm text-muted-foreground">{status}</p>
  <h1 class="mt-2 text-3xl font-semibold tracking-tight">{title}</h1>
  {#if message.toLowerCase() !== title.toLowerCase()}
    <p class="mt-4 max-w-md text-muted-foreground">{message}</p>
  {/if}
  <Link href={route('home')} class={cn(buttonVariants(), 'mt-8')}>Back home</Link>
</div>
