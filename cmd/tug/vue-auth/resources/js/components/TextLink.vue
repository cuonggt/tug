<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

// TextLink is a link within a sentence, underlined.
const props = defineProps<{ href: string; class?: HTMLAttributes['class'] }>()
</script>

<template>
  <Link
    :href="href"
    :class="
      cn(
        'text-foreground underline decoration-neutral-300 underline-offset-4 transition-colors hover:decoration-current dark:decoration-neutral-500',
        props.class,
      )
    "
  >
    <slot />
  </Link>
</template>
