import { Form, Head } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// ResetPassword is where the link in a reset mail leads: its token, and
// the email it went to, come in the props.
export default function ResetPassword({ token, email }: PageProps<'Auth/ResetPassword'>) {
  return (
    <>
      <Head title="Choose a new password" />
      <Form
        action={route('password.store')}
        method="post"
        resetOnError={['password', 'password_confirmation']}
        className="flex flex-col gap-6"
      >
        {({ errors, processing }) => (
          <>
            <input type="hidden" name="token" value={token} />
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input id="email" name="email" type="email" autoComplete="username" value={email} readOnly aria-invalid={!!errors.email} />
              <InputError message={errors.email} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password">New password</Label>
              <PasswordInput id="password" name="password" autoComplete="new-password" required autoFocus />
              <InputError message={errors.password} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="password_confirmation">New password again</Label>
              <PasswordInput id="password_confirmation" name="password_confirmation" autoComplete="new-password" required />
              <InputError message={errors.password_confirmation} />
            </div>
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              Set the password
            </Button>
          </>
        )}
      </Form>
      <p className="mt-6 text-center text-sm text-muted-foreground">
        Has the link stopped working? <TextLink href={route('password.request')}>Ask for another</TextLink>
      </p>
    </>
  )
}

ResetPassword.layout = { title: 'Choose a new password', description: 'It logs you out everywhere you were logged in.' }
