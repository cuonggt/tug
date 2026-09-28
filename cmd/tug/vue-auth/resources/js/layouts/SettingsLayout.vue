<script setup lang="ts">
import { Link, usePage } from '@inertiajs/vue3'
import Heading from '@/components/Heading.vue'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { route } from '@/tug/routes'

const pages = [
  { title: 'Profile', href: route('profile.edit') },
  { title: 'Security', href: route('security.edit') },
  { title: 'Appearance', href: route('appearance.edit') },
]

// SettingsLayout is around the settings pages, inside the app's layout:
// their names, down the side, or across the top on a phone.
const page = usePage()
</script>

<template>
  <Heading title="Settings" description="Your profile, your account's security, and how the app looks" />
  <div class="flex flex-col gap-6 lg:flex-row lg:gap-12">
    <aside class="lg:w-48">
      <nav aria-label="Settings" class="flex gap-1 lg:flex-col">
        <Button
          v-for="item in pages"
          :key="item.href"
          variant="ghost"
          size="sm"
          as-child
          :class="cn('justify-start', page.url.startsWith(item.href) && 'bg-muted')"
        >
          <Link :href="item.href">{{ item.title }}</Link>
        </Button>
      </nav>
    </aside>
    <Separator class="lg:hidden" />
    <div class="max-w-xl flex-1 space-y-12">
      <slot />
    </div>
  </div>
</template>
