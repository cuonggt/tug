<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import type { ComponentProps } from 'svelte'
  import { cn } from '@/lib/utils'

  // TextLink is a link within a sentence, underlined.
  let { class: className, children, ...props }: ComponentProps<typeof Link> = $props()
</script>

<Link
  class={cn(
    'text-foreground underline decoration-neutral-300 underline-offset-4 transition-colors hover:decoration-current dark:decoration-neutral-500',
    className,
  )}
  {...props}
>
  {@render children?.()}
</Link>
