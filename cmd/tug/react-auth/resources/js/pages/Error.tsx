import { Head, Link } from '@inertiajs/react'
import { Button } from '@/components/ui/button'
import { useLang } from '@/lib/lang'
import { text } from '@/tug/lang'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

const titles: Record<number, string> = {
  403: text('Not allowed'),
  404: text('Not found'),
  500: text('Something went wrong'),
  503: text('Back in a moment'),
}

// Error is the page tug shows errors with (Config.ErrorPage in main.go),
// with the response's own status, inside the app's layout.
export default function Error({ status, message }: PageProps<'Error'>) {
  const { t } = useLang()

  const title = t(titles[status] ?? text('Something went wrong'))
  return (
    <>
      <Head title={title} />
      <div className="flex flex-col items-center py-24 text-center">
        <p className="font-mono text-sm text-muted-foreground">{status}</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">{title}</h1>
        {message.toLowerCase() !== title.toLowerCase() && <p className="mt-4 max-w-md text-muted-foreground">{message}</p>}
        <Button className="mt-8" asChild>
          <Link href={route('home')}>{t('Back home')}</Link>
        </Button>
      </div>
    </>
  )
}
