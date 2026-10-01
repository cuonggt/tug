import { Head, router } from '@inertiajs/react'
import { RotateCcw } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// FailedJobs is the admins' page of the jobs that failed for good, which
// are kept for a month: what each was pushed with and failed with, the
// request that pushed it, whose ID its log lines have, and a button that
// runs it again from its first attempt, as the jobs command does. Its
// handlers are in admin.go, and who may see it in abilities.go.
export default function FailedJobs({ jobs }: PageProps<'Admin/FailedJobs'>) {
  return (
    <>
      <Head title="Failed jobs" />
      <div className="space-y-8">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight">Failed jobs</h1>
            <p className="text-muted-foreground">The jobs that failed for good, kept for a month, the latest first.</p>
          </div>
          {jobs.length > 0 && (
            <Button variant="outline" onClick={() => router.post(route('failed-jobs.retry-all'), {}, { preserveScroll: true })}>
              <RotateCcw />
              Run them all again
            </Button>
          )}
        </div>
        {jobs.length === 0 ? (
          <p className="text-muted-foreground">No job has failed for good.</p>
        ) : (
          <ul className="divide-y rounded-lg border">
            {jobs.map((job) => (
              <li key={job.id} className="space-y-3 p-4">
                <div className="flex items-start gap-3">
                  <div className="min-w-0 flex-1 text-sm">
                    <p className="flex flex-wrap items-center gap-2 font-medium">
                      <span className="font-mono">{job.kind}</span>
                      <Badge variant="secondary">{job.id}</Badge>
                    </p>
                    <p className="text-muted-foreground">
                      Failed {when(job.failedAt)}, after {job.attempts === 1 ? '1 attempt' : `${job.attempts} attempts`}
                      {job.request && (
                        <>
                          , pushed by request <span className="font-mono">{job.request}</span>
                        </>
                      )}
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    aria-label={`Run job ${job.id} again`}
                    onClick={() => router.post(route('failed-jobs.retry', { id: job.id }), {}, { preserveScroll: true })}
                  >
                    <RotateCcw />
                    Run again
                  </Button>
                </div>
                <pre className="overflow-x-auto rounded bg-muted px-3 py-2 font-mono text-xs">{job.payload}</pre>
                <p className="font-mono text-xs break-all text-destructive">{job.error}</p>
              </li>
            ))}
          </ul>
        )}
      </div>
    </>
  )
}

// when is a time as the page shows it, the same on the server as in any
// browser, whatever its language or zone.
function when(at: string): string {
  return `${new Date(at).toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' })} UTC`
}
