import { Form, Head } from '@inertiajs/react'
import { KeyRound, LoaderCircle, LockKeyhole } from 'lucide-react'
import { useEffect, useState } from 'react'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { passkeysWork } from '@/lib/passkeys'
import { form, route, type Inputs } from '@/tug/routes'

// Register checks each field with the server as it's left, as BindValid in
// auth.go answers: a taken email shows before the form is sent. Where the
// browser makes passkeys, the account can have one in place of a password:
// the form sends the name and the email alone, and registerWithPasskey in
// passkeys.go mails a link, whose page makes the passkey and the account.
export default function Register() {
  // Known once the page is in the browser, so the server's HTML is the same.
  const [works, setWorks] = useState(false)
  const [withPasskey, setWithPasskey] = useState(false)
  useEffect(() => setWorks(passkeysWork()), [])

  return (
    <>
      <Head title="Register" />
      <Form<Inputs['register.store']>
        action={withPasskey ? form('register.passkey') : form('register.store')}
        resetOnError={['password', 'password_confirmation']}
        validationTimeout={300}
        className="flex flex-col gap-6"
      >
        {({ errors, processing, validate, invalid }) => (
          <>
            <div className="grid gap-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                name="name"
                autoComplete="name"
                required
                autoFocus
                aria-invalid={invalid('name')}
                onBlur={() => validate('name')}
              />
              <InputError message={errors.name} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                required
                aria-invalid={invalid('email')}
                onBlur={() => validate('email')}
              />
              <InputError message={errors.email} />
            </div>
            {withPasskey ? (
              <p className="text-sm text-muted-foreground">
                We'll mail you a link: it makes your passkey, and your account with it, with no password.
              </p>
            ) : (
              <>
                <div className="grid gap-2">
                  <Label htmlFor="password">Password</Label>
                  <PasswordInput
                    id="password"
                    name="password"
                    autoComplete="new-password"
                    required
                    aria-invalid={invalid('password')}
                    onBlur={() => validate('password')}
                  />
                  <InputError message={errors.password} />
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="password_confirmation">Password again</Label>
                  <PasswordInput
                    id="password_confirmation"
                    name="password_confirmation"
                    autoComplete="new-password"
                    required
                    aria-invalid={invalid('password_confirmation')}
                    onBlur={() => validate('password_confirmation')}
                  />
                  <InputError message={errors.password_confirmation} />
                </div>
              </>
            )}
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              {withPasskey ? 'Mail me a link' : 'Register'}
            </Button>
          </>
        )}
      </Form>
      {works && (
        <Button type="button" variant="outline" className="mt-4 w-full" onClick={() => setWithPasskey(!withPasskey)}>
          {withPasskey ? <LockKeyhole /> : <KeyRound />}
          {withPasskey ? 'Use a password instead' : 'Use a passkey instead'}
        </Button>
      )}
      <p className="mt-6 text-center text-sm text-muted-foreground">
        Have an account? <TextLink href={route('login')}>Log in</TextLink>
      </p>
    </>
  )
}

Register.layout = {
  title: 'Make an account',
  description: 'Your name, your email, and a password of 8 characters or more, or a passkey.',
}
