<script setup lang="ts">
import { Link, router, usePage } from '@inertiajs/vue3'
import { Bell } from '@lucide/vue'
import { computed, onMounted, onUnmounted, watch } from 'vue'
import AppLogo from '@/components/AppLogo.vue'
import UserMenu from '@/components/UserMenu.vue'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { cn } from '@/lib/utils'
import { route } from '@/tug/routes'

// nav is the app's own pages, for users who've logged in: add each page to
// it as the app grows.
const nav = [{ title: 'Dashboard', href: route('dashboard') }]

// AppLayout is around the app's pages: its name, where to go, and who's
// logged in, or the way in for a guest, as on an error page.
const page = usePage()
const user = computed(() => page.props.auth.user)
const unread = computed(() => page.props.bell.unread)

// A notification made in another tab, or on another device, is counted
// at once: the app says so on the user's own channel, which the layout
// follows from its mount, in the browser, for as long as someone's logged
// in.
let stop: (() => void) | undefined
const follow = (id: number | undefined) => {
  stop?.()
  stop = id ? listen('notification', () => router.reload({ only: ['bell'] })) : undefined
}
onMounted(() => follow(user.value?.id))
watch(() => user.value?.id, follow)
onUnmounted(() => stop?.())
</script>

<template>
  <div class="flex min-h-svh flex-col">
    <header class="border-b">
      <div class="mx-auto flex h-14 w-full max-w-6xl items-center gap-6 px-4">
        <Link :href="user ? route('dashboard') : route('home')" class="rounded-md">
          <AppLogo />
        </Link>
        <nav v-if="user" aria-label="Main" class="flex items-center gap-1 text-sm">
          <Link
            v-for="item in nav"
            :key="item.href"
            :href="item.href"
            :class="
              cn(
                'rounded-md px-3 py-1.5 text-muted-foreground transition-colors hover:text-foreground',
                page.url.startsWith(item.href) && 'bg-accent text-accent-foreground',
              )
            "
          >
            {{ item.title }}
          </Link>
        </nav>
        <div class="ml-auto flex items-center gap-2">
          <template v-if="user">
            <Button variant="ghost" size="icon" class="relative" as-child>
              <Link :href="route('notifications.index')" :aria-label="unread > 0 ? `Notifications, ${unread} unread` : 'Notifications'">
                <Bell />
                <span
                  v-if="unread > 0"
                  class="absolute top-1 right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-medium text-primary-foreground"
                >
                  {{ unread > 99 ? '99+' : unread }}
                </span>
              </Link>
            </Button>
            <UserMenu :user="user" />
          </template>
          <template v-else>
            <Button variant="ghost" size="sm" as-child>
              <Link :href="route('login')">Log in</Link>
            </Button>
            <Button size="sm" as-child>
              <Link :href="route('register')">Register</Link>
            </Button>
          </template>
        </div>
      </div>
    </header>
    <main class="mx-auto w-full max-w-6xl flex-1 px-4 py-8">
      <slot />
    </main>
  </div>
</template>
