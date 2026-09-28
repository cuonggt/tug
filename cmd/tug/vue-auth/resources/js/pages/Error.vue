<script setup lang="ts">
import { Head, Link } from '@inertiajs/vue3'
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

const titles: Record<number, string> = {
  403: 'Not allowed',
  404: 'Not found',
  500: 'Something went wrong',
  503: 'Back in a moment',
}

// Error is the page tug shows errors with (Config.ErrorPage in main.go),
// with the response's own status, inside the app's layout.
const props = defineProps<Pages['Error'] & SharedProps>()
const title = computed(() => titles[props.status] ?? 'Something went wrong')
</script>

<template>
  <Head :title="title" />
  <div class="flex flex-col items-center py-24 text-center">
    <p class="font-mono text-sm text-muted-foreground">{{ status }}</p>
    <h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ title }}</h1>
    <p v-if="message.toLowerCase() !== title.toLowerCase()" class="mt-4 max-w-md text-muted-foreground">{{ message }}</p>
    <Button class="mt-8" as-child>
      <Link :href="route('home')">Back home</Link>
    </Button>
  </div>
</template>
