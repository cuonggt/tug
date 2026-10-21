<script setup lang="ts">
import { Head, Link } from '@inertiajs/vue3'
import { computed } from 'vue'
import { Button } from '@/components/ui/button'
  import { useLang } from '@/lib/lang'
  import { text } from '@/tug/lang'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

const titles: Record<number, string> = {
  403: text('Not allowed'),
  404: text('Not found'),
  500: text('Something went wrong'),
  503: text('Back in a moment'),
}

// Error is the page tug shows errors with (Config.ErrorPage in main.go),
// with the response's own status, inside the app's layout.
const props = defineProps<Pages['Error'] & SharedProps>()
const title = computed(() => t(titles[props.status] ?? text('Something went wrong')))
</script>

<template>
  <Head :title="title" />
  <div class="flex flex-col items-center py-24 text-center">
    <p class="font-mono text-sm text-muted-foreground">{{ status }}</p>
    <h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ title }}</h1>
    <p v-if="message.toLowerCase() !== title.toLowerCase()" class="mt-4 max-w-md text-muted-foreground">{{ message }}</p>
    <Button class="mt-8" as-child>
      <Link :href="route('home')">{{ t('Back home') }}</Link>
    </Button>
  </div>
</template>
