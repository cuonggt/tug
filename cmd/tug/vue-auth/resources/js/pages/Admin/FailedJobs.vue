<script setup lang="ts">
import { Head, router } from '@inertiajs/vue3'
import { RotateCcw } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// FailedJobs is the admins' page of the jobs that failed for good, which
// are kept for a month: what each was pushed with and failed with, the
// request that pushed it, whose ID its log lines have, and a button that
// runs it again from its first attempt, as the jobs command does. Its
// handlers are in admin.go, and who may see it in abilities.go.
defineProps<Pages['Admin/FailedJobs'] & SharedProps>()

// when is a time as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function when(at: string): string {
  return `${new Date(at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' })} UTC`
}
</script>

<template>
  <Head title="Failed jobs" />
  <div class="space-y-8">
    <div class="flex flex-wrap items-end justify-between gap-4">
      <div class="space-y-1">
        <h1 class="text-2xl font-semibold tracking-tight">Failed jobs</h1>
        <p class="text-muted-foreground">The jobs that failed for good, kept for a month, the latest first.</p>
      </div>
      <Button v-if="jobs.length > 0" variant="outline" @click="router.post(route('failed-jobs.retry-all'), {}, { preserveScroll: true })">
        <RotateCcw />
        Run them all again
      </Button>
    </div>
    <p v-if="jobs.length === 0" class="text-muted-foreground">No job has failed for good.</p>
    <ul v-else class="divide-y rounded-lg border">
      <li v-for="job in jobs" :key="job.id" class="space-y-3 p-4">
        <div class="flex items-start gap-3">
          <div class="min-w-0 flex-1 text-sm">
            <p class="flex flex-wrap items-center gap-2 font-medium">
              <span class="font-mono">{{ job.kind }}</span>
              <!-- A span, as shadcn's React Badge is: a div can't be in a p. -->
              <Badge as="span" variant="secondary">{{ job.id }}</Badge>
            </p>
            <p class="text-muted-foreground">
              <!-- The request's on the same line: a line break would be a space. -->
              Failed {{ when(job.failedAt) }}, after {{ job.attempts === 1 ? '1 attempt' : `${job.attempts} attempts` }}<template v-if="job.request">, pushed by request <span class="font-mono">{{ job.request }}</span></template>
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            :aria-label="`Run job ${job.id} again`"
            @click="router.post(route('failed-jobs.retry', { id: job.id }), {}, { preserveScroll: true })"
          >
            <RotateCcw />
            Run again
          </Button>
        </div>
        <pre class="overflow-x-auto rounded bg-muted px-3 py-2 font-mono text-xs">{{ job.payload }}</pre>
        <p class="font-mono text-xs break-all text-destructive">{{ job.error }}</p>
      </li>
    </ul>
  </div>
</template>
