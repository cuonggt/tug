<script setup lang="ts">
import { Head } from '@inertiajs/vue3'
import AppearanceTabs from '@/components/AppearanceTabs.vue'
import Heading from '@/components/Heading.vue'

// Appearance picks light, dark, or the system's, for this browser.
</script>

<template>
  <Head title="Appearance" />
  <section class="space-y-6">
    <Heading small title="Appearance" description="How the app looks in this browser." />
    <AppearanceTabs />
  </section>
</template>
