import { Form, Head } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import InputError from '@/components/input-error'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { route } from '@/tug/routes'

// ForgotPassword mails a link that sets a new password. In development,
// without MAIL_HOST in .env, the mail is written to tug dev's terminal.
export default function ForgotPassword() {
  return (
    <>
      <Head title="Forgotten password" />
      <Form action={route('password.email')} method="post" resetOnSuccess className="flex flex-col gap-6">
        {({ errors, processing }) => (
          <>
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                required
                autoFocus
                aria-invalid={!!errors.email}
              />
              <InputError message={errors.email} />
            </div>
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              Mail me a link
            </Button>
          </>
        )}
      </Form>
      <p className="mt-6 text-center text-sm text-muted-foreground">
        Remembered it? <TextLink href={route('login')}>Log in</TextLink>
      </p>
    </>
  )
}

ForgotPassword.layout = {
  title: 'Forgotten your password?',
  description: "Say the email of your account, and we'll mail you a link to choose a new one.",
}
