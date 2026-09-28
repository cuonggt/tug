<script setup lang="ts">
import { Head, Link } from '@inertiajs/vue3'
import { computed } from 'vue'
import Layout from '../Layout.vue'
import type { Pages, SharedProps } from '../tug/pages'
import { route } from '../tug/routes'

const titles: Record<number, string> = {
  403: 'Not allowed',
  404: 'Not found',
  500: 'Something went wrong',
}

// Error is the page tug shows errors with (Config.ErrorPage in main.go),
// with the response's own status.
const props = defineProps<Pages['Error'] & SharedProps>()
const title = computed(() => titles[props.status] ?? 'Something went wrong')
</script>

<template>
  <Layout>
    <Head :title="title" />
    <h1>{{ title }}</h1>
    <p>{{ status }}: {{ message }}</p>
    <p>
      <Link :href="route('home')">Back home</Link>
    </p>
  </Layout>
</template>
