import { Form, Head, Link, router, usePage } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import { useEffect } from 'react'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { route } from '@/tug/routes'

// VerifyEmail is where the pages for verified users send someone who
// hasn't followed the link mailed to them yet. In development, without
// MAIL_HOST in .env, the mail is written to tug dev's terminal.
export default function VerifyEmail() {
  const { auth } = usePage().props
  // Verified in another tab, or on the phone the mail went to, the email
  // moves this page on: the app says so on the user's own channel, which
  // /broadcasts is, in broadcasts.go. The browser connects again when the
  // stream ends, as when the app restarts, and the page reloads then, as it
  // may have missed the event: the email verified, it moves on.
  useEffect(() => {
    const events = new EventSource(route('broadcasts'))
    let opened = false
    events.addEventListener('open', () => {
      if (opened) router.reload()
      opened = true
    })
    events.addEventListener('verified', () => router.visit(route('dashboard')))
    return () => events.close()
  }, [])
  return (
    <>
      <Head title="Verify your email" />
      <div className="flex flex-col gap-4 text-center">
        <p className="text-sm text-muted-foreground">
          We mailed a link to <span className="font-medium text-foreground">{auth.user?.email}</span>. Follow it to
          verify the email is yours; it works for a day.
        </p>
        <Form action={route('verification.send')} method="post">
          {({ processing }) => (
            <Button type="submit" variant="secondary" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              Send another link
            </Button>
          )}
        </Form>
        <p className="text-sm text-muted-foreground">
          The wrong email? <TextLink href={route('profile.edit')}>Change it</TextLink>, or{' '}
          <Link href={route('logout')} method="post" as="button" className="cursor-pointer underline underline-offset-4">
            log out
          </Link>
          .
        </p>
      </div>
    </>
  )
}

VerifyEmail.layout = { title: 'Verify your email', description: 'One more step, and the app is yours to use.' }
