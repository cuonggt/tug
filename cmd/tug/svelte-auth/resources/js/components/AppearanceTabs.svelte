<script lang="ts">
  import type { LucideIcon } from '@lucide/svelte'
  import Monitor from '@lucide/svelte/icons/monitor'
  import Moon from '@lucide/svelte/icons/moon'
  import Sun from '@lucide/svelte/icons/sun'
  import { appearance, type Appearance } from '@/lib/appearance.svelte'
  import { cn } from '@/lib/utils'

  const choices: { value: Appearance; icon: LucideIcon; label: string }[] = [
    { value: 'light', icon: Sun, label: 'Light' },
    { value: 'dark', icon: Moon, label: 'Dark' },
    { value: 'system', icon: Monitor, label: 'System' },
  ]

  // AppearanceTabs choose light, dark, or the system's appearance.
</script>

<div role="radiogroup" aria-label="Appearance" class="inline-flex gap-1 rounded-lg bg-muted p-1">
  {#each choices as { value, icon: Icon, label } (value)}
    <button
      type="button"
      role="radio"
      aria-checked={appearance.current === value}
      onclick={() => appearance.set(value)}
      class={cn(
        'flex items-center gap-1.5 rounded-md px-3.5 py-1.5 text-sm transition-colors',
        appearance.current === value
          ? 'bg-background text-foreground shadow-xs'
          : 'text-muted-foreground hover:bg-background/60 hover:text-foreground',
      )}
    >
      <Icon class="size-4" />
      {label}
    </button>
  {/each}
</div>
