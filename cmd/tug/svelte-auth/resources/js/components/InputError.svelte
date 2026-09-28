<script lang="ts">
  import { cn } from '@/lib/utils'

  // InputError is what the server said is wrong with a field, under it.
  let { message, class: className }: { message?: string; class?: string } = $props()
</script>

{#if message}
  <p class={cn('text-sm text-destructive', className)}>{message}</p>
{/if}
