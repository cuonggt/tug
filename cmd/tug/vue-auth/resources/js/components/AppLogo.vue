<script setup lang="ts">
import { usePage } from '@inertiajs/vue3'
import AppLogoIcon from '@/components/AppLogoIcon.vue'

// AppLogo is the mark and the name, as the header has them.
const page = usePage()
</script>

<template>
  <span class="flex items-center gap-2">
    <AppLogoIcon />
    <span class="font-semibold tracking-tight">{{ page.props.appName }}</span>
  </span>
</template>
