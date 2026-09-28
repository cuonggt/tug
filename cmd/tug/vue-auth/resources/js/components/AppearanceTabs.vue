<script setup lang="ts">
import { type LucideIcon, Monitor, Moon, Sun } from '@lucide/vue'
import { type Appearance, useAppearance } from '@/composables/useAppearance'
import { cn } from '@/lib/utils'

const choices: { value: Appearance; icon: LucideIcon; label: string }[] = [
  { value: 'light', icon: Sun, label: 'Light' },
  { value: 'dark', icon: Moon, label: 'Dark' },
  { value: 'system', icon: Monitor, label: 'System' },
]

// AppearanceTabs choose light, dark, or the system's appearance.
const { appearance, setAppearance } = useAppearance()
</script>

<template>
  <div role="radiogroup" aria-label="Appearance" class="inline-flex gap-1 rounded-lg bg-muted p-1">
    <button
      v-for="{ value, icon, label } in choices"
      :key="value"
      type="button"
      role="radio"
      :aria-checked="appearance === value"
      :class="
        cn(
          'flex items-center gap-1.5 rounded-md px-3.5 py-1.5 text-sm transition-colors',
          appearance === value
            ? 'bg-background text-foreground shadow-xs'
            : 'text-muted-foreground hover:bg-background/60 hover:text-foreground',
        )
      "
      @click="setAppearance(value)"
    >
      <component :is="icon" class="size-4" />{{ label }}
    </button>
  </div>
</template>
