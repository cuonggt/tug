<script setup lang="ts">
import { Link, usePage } from '@inertiajs/vue3'

// Layout is what every page has around it: the app's name, and the flash
// message the page after a form arrives with.
const page = usePage()
</script>

<template>
  <main>
    <p class="brand">
      <Link href="/">{{ page.props.appName }}</Link>
    </p>
    <p v-if="page.flash.success" role="status" class="flash">
      {{ page.flash.success }}
    </p>
    <slot />
  </main>
</template>
