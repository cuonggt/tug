<script setup lang="ts">
import { type LucideIcon, Monitor, Moon, Sun } from '@lucide/vue'
import { type Appearance, useAppearance } from '@/composables/useAppearance'
import { useLang } from '@/lib/lang'
import { cn } from '@/lib/utils'
  import { text } from '@/tug/lang'

// The page says its own words in the request's language.
const { t } = useLang()

const choices: { value: Appearance; icon: LucideIcon; label: string }[] = [
  { value: 'light', icon: Sun, label: text('Light') },
  { value: 'dark', icon: Moon, label: text('Dark') },
  { value: 'system', icon: Monitor, label: text('System') },
]

// AppearanceTabs choose light, dark, or the system's appearance.
const { appearance, setAppearance } = useAppearance()
</script>

<template>
  <div role="radiogroup" :aria-label="t('Appearance')" class="inline-flex gap-1 rounded-lg bg-muted p-1">
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
      <component :is="icon" class="size-4" />{{ t(label) }}
    </button>
  </div>
</template>
