<script setup lang="ts">
import { Head } from '@inertiajs/vue3'
import AppearanceTabs from '@/components/AppearanceTabs.vue'
import Heading from '@/components/Heading.vue'
  import { useLang } from '@/lib/lang'

  // The page says its own words in the request's language.
  const { t } = useLang()

// Appearance picks light, dark, or the system's, for this browser.
</script>

<template>
  <Head :title="t('Appearance')" />
  <section class="space-y-6">
    <Heading small :title="t('Appearance')" :description="t('How the app looks in this browser.')" />
    <AppearanceTabs />
  </section>
</template>
