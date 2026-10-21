import { Form, Head, router } from '@inertiajs/react'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import LoginSettings from '@/components/login-settings'
import PasskeySettings from '@/components/passkey-settings'
import PasswordInput from '@/components/password-input'
import TwoFactorSettings from '@/components/two-factor-settings'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { useLang } from '@/lib/lang'
import type { PageProps } from '@/tug/pages'
import { form, route, type Inputs } from '@/tug/routes'

// Security changes the user's password, which logs them out everywhere
// else, or sets one, for an account made with a passkey, or removes it,
// for one with a passkey to log in with; turns two-factor logins on and
// off, keeps their passkeys, and lists the browsers they're logged in
// from.
export default function Security(props: PageProps<'Settings/Security'>) {
  const { t } = useLang()

  const { user, passkeys } = props
  return (
    <>
      <Head title={t('Security')} />
      <section className="space-y-6">
        <Heading
          small
          title={t('Password')}
          description={
            user.password
              ? t('A new one logs you out everywhere else, such as a lost laptop.') : t('Your account logs in with its passkeys alone. A password logs in too, and logs you out everywhere else.')
          }
        />
        <Form<Inputs['user-password.update']>
          action={form('user-password.update')}
          options={{ preserveScroll: true }}
          resetOnError
          resetOnSuccess
          className="space-y-6"
        >
          {({ errors, processing }) => (
            <>
              {user.password && (
                <div className="grid gap-2">
                  <Label htmlFor="current_password">{t('Current password')}</Label>
                  <PasswordInput id="current_password" name="current_password" autoComplete="current-password" required />
                  <InputError message={errors.current_password} />
                </div>
              )}
              <div className="grid gap-2">
                <Label htmlFor="password">{t('New password')}</Label>
                <PasswordInput id="password" name="password" autoComplete="new-password" required />
                <InputError message={errors.password} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="password_confirmation">{t('New password again')}</Label>
                <PasswordInput id="password_confirmation" name="password_confirmation" autoComplete="new-password" required />
                <InputError message={errors.password_confirmation} />
              </div>
              <Button type="submit" disabled={processing}>
                {user.password ? t('Change the password') : t('Set a password')}
              </Button>
            </>
          )}
        </Form>
        {user.password && passkeys.length > 0 && (
          <div className="space-y-2">
            <p className="text-sm text-muted-foreground">{t('Your passkeys log you in without it.')}</p>
            <Button variant="outline" onClick={() => router.delete(route('user-password.destroy'), { preserveScroll: true })}>
              {t('Remove the password')}
            </Button>
          </div>
        )}
      </section>
      <PasskeySettings {...props} />
      <TwoFactorSettings {...props} />
      <LoginSettings {...props} />
    </>
  )
}
