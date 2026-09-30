<script module lang="ts">
  // when is a time as the page shows it, the same on the server as in any
  // browser, whatever its language or zone.
  function when(at: string): string {
    return `${new Date(at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' })} UTC`
  }
</script>

<script lang="ts">
  import { router } from '@inertiajs/svelte'
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw'
  import Head from '@/Head.svelte'
  import { Badge } from '@/components/ui/badge'
  import { Button } from '@/components/ui/button'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // FailedJobs is the admins' page of the jobs that failed for good, which
  // are kept for a month: what each was pushed with and failed with, and a
  // button that runs it again from its first attempt, as the jobs command
  // does. Its handlers are in admin.go, and who may see it in abilities.go.
  let { jobs }: PageProps<'Admin/FailedJobs'> = $props()
</script>

<Head title="Failed jobs" />
<div class="space-y-8">
  <div class="flex flex-wrap items-end justify-between gap-4">
    <div class="space-y-1">
      <h1 class="text-2xl font-semibold tracking-tight">Failed jobs</h1>
      <p class="text-muted-foreground">The jobs that failed for good, kept for a month, the latest first.</p>
    </div>
    {#if jobs.length > 0}
      <Button variant="outline" onclick={() => router.post(route('failed-jobs.retry-all'), {}, { preserveScroll: true })}>
        <RotateCcw />
        Run them all again
      </Button>
    {/if}
  </div>
  {#if jobs.length === 0}
    <p class="text-muted-foreground">No job has failed for good.</p>
  {:else}
    <ul class="divide-y rounded-lg border">
      {#each jobs as job (job.id)}
        <li class="space-y-3 p-4">
          <div class="flex items-start gap-3">
            <div class="min-w-0 flex-1 text-sm">
              <p class="flex flex-wrap items-center gap-2 font-medium">
                <span class="font-mono">{job.kind}</span>
                <Badge variant="secondary">{job.id}</Badge>
              </p>
              <p class="text-muted-foreground">
                Failed {when(job.failedAt)}, after {job.attempts === 1 ? '1 attempt' : `${job.attempts} attempts`}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              aria-label={`Run job ${job.id} again`}
              onclick={() => router.post(route('failed-jobs.retry', { id: job.id }), {}, { preserveScroll: true })}
            >
              <RotateCcw />
              Run again
            </Button>
          </div>
          <pre class="overflow-x-auto rounded bg-muted px-3 py-2 font-mono text-xs">{job.payload}</pre>
          <p class="font-mono text-xs break-all text-destructive">{job.error}</p>
        </li>
      {/each}
    </ul>
  {/if}
</div>
