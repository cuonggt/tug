<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

// InputError is what the server said is wrong with a field, under it.
const props = defineProps<{ message?: string; class?: HTMLAttributes['class'] }>()
</script>

<template>
  <p v-if="message" :class="cn('text-sm text-destructive', props.class)">{{ message }}</p>
</template>
