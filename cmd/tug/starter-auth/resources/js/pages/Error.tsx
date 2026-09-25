import { Head, Link } from '@inertiajs/react'
import { Button } from '@/components/ui/button'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

const titles: Record<number, string> = {
  403: 'Not allowed',
  404: 'Not found',
  500: 'Something went wrong',
  503: 'Back in a moment',
}

// Error is the page tug shows errors with (Config.ErrorPage in main.go),
// with the response's own status, inside the app's layout.
export default function Error({ status, message }: PageProps<'Error'>) {
  const title = titles[status] ?? 'Something went wrong'
  return (
    <>
      <Head title={title} />
      <div className="flex flex-col items-center py-24 text-center">
        <p className="font-mono text-sm text-muted-foreground">{status}</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">{title}</h1>
        {message.toLowerCase() !== title.toLowerCase() && <p className="mt-4 max-w-md text-muted-foreground">{message}</p>}
        <Button className="mt-8" asChild>
          <Link href={route('home')}>Back home</Link>
        </Button>
      </div>
    </>
  )
}
