<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import AppLogoIcon from '@/components/AppLogoIcon.vue'
import { route } from '@/tug/routes'

// AuthLayout is around logging in, registering and the rest of Auth/: the
// app's mark, the page's title and what to do, and a card for the form.
// A page names them with a layout option of its own, as Login's
// defineOptions({ layout: { title: 'Log in', description: '...' } }).
defineProps<{ title?: string; description?: string }>()
</script>

<template>
  <div class="flex min-h-svh flex-col items-center justify-center bg-muted/40 p-6 md:p-10">
    <div class="flex w-full max-w-sm flex-col gap-6">
      <Link :href="route('home')" aria-label="Home" class="self-center rounded-md">
        <AppLogoIcon class="size-10 text-base" />
      </Link>
      <div class="rounded-xl border bg-card p-6 text-card-foreground shadow-xs sm:p-8">
        <div class="mb-6 space-y-1.5 text-center">
          <h1 class="text-xl font-semibold tracking-tight">{{ title }}</h1>
          <p v-if="description" class="text-sm text-balance text-muted-foreground">{{ description }}</p>
        </div>
        <slot />
      </div>
    </div>
  </div>
</template>
