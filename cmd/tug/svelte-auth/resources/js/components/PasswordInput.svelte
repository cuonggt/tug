<script lang="ts">
  import Eye from '@lucide/svelte/icons/eye'
  import EyeOff from '@lucide/svelte/icons/eye-off'
  import type { HTMLInputAttributes } from 'svelte/elements'
  import { Input } from '@/components/ui/input'
  import { cn, type WithElementRef } from '@/lib/utils'

  // PasswordInput is a password field with a button that shows what's been
  // typed, to check it on a phone's keyboard.
  let {
    ref = $bindable(null),
    class: className,
    ...props
  }: WithElementRef<Omit<HTMLInputAttributes, 'type' | 'files'>> = $props()
  let shown = $state(false)
</script>

<div class="relative">
  <Input bind:ref type={shown ? 'text' : 'password'} class={cn('pr-10', className)} {...props} />
  <button
    type="button"
    onclick={() => (shown = !shown)}
    aria-label={shown ? 'Hide the password' : 'Show the password'}
    tabindex={-1}
    class="absolute inset-y-0 right-0 flex items-center rounded-r-md px-3 text-muted-foreground hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
  >
    {#if shown}
      <EyeOff class="size-4" />
    {:else}
      <Eye class="size-4" />
    {/if}
  </button>
</div>
