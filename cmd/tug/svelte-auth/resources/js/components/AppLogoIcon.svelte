<script lang="ts">
  import { page } from '@inertiajs/svelte'
  import { cn } from '@/lib/utils'

  // AppLogoIcon is the app's mark: the first letter of its name, until it
  // has a logo of its own to put here.
  let { class: className }: { class?: string } = $props()
</script>

<span
  aria-hidden="true"
  class={cn(
    'flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground',
    className,
  )}
>
  {page.props.appName.charAt(0).toUpperCase()}
</span>
