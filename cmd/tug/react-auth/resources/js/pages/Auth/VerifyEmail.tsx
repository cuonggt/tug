import { Form, Head, Link, router, usePage } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import { useEffect } from 'react'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { listen } from '@/lib/broadcasts'
import { route } from '@/tug/routes'

// VerifyEmail is where the pages for verified users send someone who
// hasn't followed the link mailed to them yet. In development, without
// MAIL_HOST in .env, the mail is written to tug dev's terminal.
export default function VerifyEmail() {
  const { auth } = usePage().props
  // Verified in another tab, or on the phone the mail went to, the email
  // moves this page on: the app says so on the user's own channel, and the
  // page reloads, which its handler, in verify.go, answers with the
  // dashboard once the email is verified.
  useEffect(() => listen('verified', () => router.reload()), [])
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
