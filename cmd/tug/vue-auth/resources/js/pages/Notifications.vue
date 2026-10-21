<script setup lang="ts">
import { Head, Link, router } from '@inertiajs/vue3'
import { onMounted, onUnmounted } from 'vue'
import { Button } from '@/components/ui/button'
import { useLang } from '@/lib/lang'
import { listen } from '@/lib/broadcasts'
import { cn } from '@/lib/utils'
import type { Pages, SharedProps } from '@/tug/pages'

// The page says its own words in the request's language.
const { t, d } = useLang()

// Notifications is what happened to the user's account, the newest first,
// a page at a time, which its handler, in notifications.go, marks read as
// it shows them: the ones new to this page have a dot. A notification made
// while it's open shows at once.
defineProps<Pages['Notifications'] & SharedProps>()

let stop: (() => void) | undefined
onMounted(() => (stop = listen('notification', () => router.reload({ only: ['list', 'bell'] }))))
onUnmounted(() => stop?.())

// when is a time as the page shows it, with the name of the zone it's in:
// the app's own, and the reader's once the page is in their browser.
const when = (at: string) =>
  d(at, { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit', timeZoneName: 'short' })
</script>

<template>
  <Head :title="t('Notifications')" />
  <div class="space-y-8">
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold tracking-tight">{{ t('Notifications') }}</h1>
      <p class="text-muted-foreground">{{ t('What happened to your account, the newest first.') }}</p>
    </div>
    <p v-if="list.data.length === 0" class="text-muted-foreground">{{ t('Nothing has happened yet.') }}</p>
    <ul v-else class="divide-y rounded-lg border">
      <li v-for="n in list.data" :key="n.id">
        <Link :href="n.link" class="flex items-start gap-3 p-4 transition-colors hover:bg-accent/50">
          <span :class="cn('mt-1.5 size-2 shrink-0 rounded-full', n.new && 'bg-primary')" />
          <span class="min-w-0 flex-1">
            <span :class="cn('block text-sm', n.new && 'font-medium')">
              <span v-if="n.new" class="sr-only">{{ t('New:') }} </span>{{ n.line }}
            </span>
            <span class="block text-xs text-muted-foreground">{{ when(n.createdAt) }}</span>
          </span>
        </Link>
      </li>
    </ul>
    <nav v-if="list.prev_page_url || list.next_page_url" :aria-label="t('Pages')" class="flex justify-between">
      <Button v-if="list.prev_page_url" variant="outline" size="sm" as-child>
        <Link :href="list.prev_page_url">{{ t('Newer') }}</Link>
      </Button>
      <span v-else />
      <Button v-if="list.next_page_url" variant="outline" size="sm" as-child>
        <Link :href="list.next_page_url">{{ t('Older') }}</Link>
      </Button>
    </nav>
  </div>
</template>
