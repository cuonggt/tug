import { Form, Head, router, usePage } from '@inertiajs/react'
import { KeyRound, LoaderCircle } from 'lucide-react'
import { useEffect, useState } from 'react'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { useLang } from '@/lib/lang'
import { dismissed, getPasskey, optionsFrom, passkeysWork } from '@/lib/passkeys'
import { text } from '@/tug/lang'
import type { PageProps } from '@/tug/pages'
import { form, route, type Inputs } from '@/tug/routes'

// ConfirmPassword asks for the password again before the settings that
// could hand the account to someone else: passwordConfirmed in auth.go
// sends users here when they last typed it over three hours ago. One of
// their passkeys says it's them as well, and alone for an account with no
// password.
export default function ConfirmPassword({ password, passkeys }: PageProps<'Auth/ConfirmPassword'>) {
  const { t } = useLang()

  const { errors } = usePage().props
  const [works, setWorks] = useState(false)
  const [failure, setFailure] = useState<string>()
  useEffect(() => setWorks(passkeysWork()), [])

  const withPasskey = async () => {
    setFailure(undefined)
    try {
      const options = await optionsFrom(route('password.confirm.passkey.options'))
      if (options) router.post(route('password.confirm.passkey'), { credential: await getPasskey(options) })
    } catch (err) {
      if (!dismissed(err))
        setFailure(password ? "Your browser didn't use a passkey: try again, or type your password." : "Your browser didn't use a passkey: try again.")
    }
  }

  return (
    <>
      <Head title={t("Confirm it's you")} />
      {password && (
        <Form<Inputs['password.confirm.store']> action={form('password.confirm.store')} resetOnError className="flex flex-col gap-6">
          {({ errors, processing }) => (
            <>
              <div className="grid gap-2">
                <Label htmlFor="password">{t('Password')}</Label>
                <PasswordInput id="password" name="password" autoComplete="current-password" required autoFocus />
                <InputError message={errors.password} />
              </div>
              <Button type="submit" className="w-full" disabled={processing}>
                {processing && <LoaderCircle className="animate-spin" />}
                {t('Confirm')}
              </Button>
            </>
          )}
        </Form>
      )}
      {passkeys && works && (
        <div className={password ? 'mt-4 grid gap-2' : 'grid gap-2'}>
          <Button type="button" variant={password ? 'outline' : 'default'} className="w-full" onClick={withPasskey}>
            <KeyRound />
            {t('Use a passkey')}
          </Button>
          <InputError message={errors.passkey ?? failure} />
        </div>
      )}
    </>
  )
}

ConfirmPassword.layout = {
  title: text("Confirm it's you"),
  description: text("You're on your way to your account's settings, which ask for your password or a passkey once in a while."),
}
