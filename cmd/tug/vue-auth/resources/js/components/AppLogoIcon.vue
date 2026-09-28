<script setup lang="ts">
import { usePage } from '@inertiajs/vue3'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

// AppLogoIcon is the app's mark: the first letter of its name, until it
// has a logo of its own to put here.
const props = defineProps<{ class?: HTMLAttributes['class'] }>()
const page = usePage()
</script>

<template>
  <span
    aria-hidden="true"
    :class="
      cn(
        'flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground',
        props.class,
      )
    "
  >
    {{ page.props.appName.charAt(0).toUpperCase() }}
  </span>
</template>
