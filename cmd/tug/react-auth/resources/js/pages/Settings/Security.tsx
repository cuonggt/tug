import { Form, Head } from '@inertiajs/react'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import LoginSettings from '@/components/login-settings'
import PasskeySettings from '@/components/passkey-settings'
import PasswordInput from '@/components/password-input'
import TwoFactorSettings from '@/components/two-factor-settings'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import type { PageProps } from '@/tug/pages'
import { form, type Inputs } from '@/tug/routes'

// Security changes the user's password, which logs them out everywhere
// else, turns two-factor logins on and off, keeps their passkeys, and
// lists the browsers they're logged in from.
export default function Security(props: PageProps<'Settings/Security'>) {
  return (
    <>
      <Head title="Security" />
      <section className="space-y-6">
        <Heading small title="Password" description="A new one logs you out everywhere else, such as a lost laptop." />
        <Form<Inputs['user-password.update']>
          action={form('user-password.update')}
          options={{ preserveScroll: true }}
          resetOnError
          resetOnSuccess
          className="space-y-6"
        >
          {({ errors, processing }) => (
            <>
              <div className="grid gap-2">
                <Label htmlFor="current_password">Current password</Label>
                <PasswordInput id="current_password" name="current_password" autoComplete="current-password" required />
                <InputError message={errors.current_password} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password">New password</Label>
                <PasswordInput id="password" name="password" autoComplete="new-password" required />
                <InputError message={errors.password} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password_confirmation">New password again</Label>
                <PasswordInput id="password_confirmation" name="password_confirmation" autoComplete="new-password" required />
                <InputError message={errors.password_confirmation} />
              </div>
              <Button type="submit" disabled={processing}>
                Change the password
              </Button>
            </>
          )}
        </Form>
      </section>
      <PasskeySettings {...props} />
      <TwoFactorSettings {...props} />
      <LoginSettings {...props} />
    </>
  )
}
