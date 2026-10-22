<script setup lang="ts">
import { router } from '@inertiajs/vue3'
import { LogOut, Monitor } from '@lucide/vue'
import { onMounted, onUnmounted } from 'vue'
import Heading from '@/components/Heading.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { useLang } from '@/lib/lang'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// The page says its own words in the request's language.
const { t, d } = useLang()

// LoginSettings lists the browsers the user is logged in from, this one
// first, and logs one of the others out, or all of them: a browser left
// logged in on a lost laptop, say. The handlers are in logins.go.
defineProps<Pages['Settings/Security'] & SharedProps>()

// capital is a browser's name as a line starts with it: "a browser on
// Linux", one the app doesn't know, as "A browser on Linux".
function capital(name: string): string {
  return name.charAt(0).toUpperCase() + name.slice(1)
}

// day is a date as the page shows it, in the page's own language, and in
// the reader's own zone once the page is in their browser.
const day = (at: string) => d(at, { dateStyle: 'medium' })

// A browser of the account opening or closing shows at once: each page is
// on the user's channel while it's open, and the app says as one comes or
// goes.
let stops: (() => void)[] = []
onMounted(() => {
  const reload = () => router.reload({ only: ['browsers'] })
  stops = [listen('presence.joined', reload), listen('presence.left', reload)]
})
onUnmounted(() => stops.forEach((stop) => stop()))
</script>

<template>
  <section class="space-y-6">
    <Heading small :title="t('Browsers')" :description="t('Where you\'re logged in. Log out of one you don\'t know, or of every one but this.')" />
    <ul v-if="browsers.length > 0" class="divide-y rounded-lg border">
      <li v-for="browser in browsers" :key="browser.id" class="flex items-center gap-3 p-3">
        <Monitor class="size-4 shrink-0 text-muted-foreground" />
        <div class="min-w-0 flex-1 text-sm">
          <p class="flex items-center gap-2 font-medium">
            <span class="truncate">{{ capital(browser.name) }}</span>
            <!-- A span, as shadcn's React Badge is: a div can't be in a p. -->
            <Badge v-if="browser.current" as="span" variant="secondary">{{ t('This browser') }}</Badge>
          </p>
          <p class="text-muted-foreground">
            {{ browser.ip }}, {{ t('logged in :date', { date: day(browser.began) }) }},
            {{ browser.open ? t('open now') : t('last seen :date', { date: day(browser.lastSeen) }) }}
          </p>
        </div>
        <Button
          v-if="!browser.current"
          variant="ghost"
          size="icon"
          :aria-label="t('Log out of :name', { name: browser.name })"
          @click="router.delete(route('logins.destroy', { id: browser.id }), { preserveScroll: true })"
        >
          <LogOut />
        </Button>
      </li>
    </ul>
    <Button v-if="browsers.length > 1" variant="outline" @click="router.delete(route('logins.destroy-others'), { preserveScroll: true })">
      <LogOut />{{ t('Log out of every other browser') }}
    </Button>
  </section>
</template>
