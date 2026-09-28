import { Form, Head } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import { useState } from 'react'
import InputError from '@/components/input-error'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { route } from '@/tug/routes'

// TwoFactorChallenge is the second step of logging in, for a user who has
// turned two-factor logins on: a code from their authenticator app, or,
// with the phone lost, one of their recovery codes.
export default function TwoFactorChallenge() {
  const [recovery, setRecovery] = useState(false)
  return (
    <>
      <Head title="Two-factor login" />
      <Form
        key={recovery ? 'recovery' : 'code'}
        action={route('two-factor.login.store')}
        method="post"
        resetOnError
        className="flex flex-col gap-6"
      >
        {({ errors, processing }) => (
          <>
            {recovery ? (
              <div className="grid gap-2">
                <Label htmlFor="recovery_code">Recovery code</Label>
                <Input
                  id="recovery_code"
                  name="recovery_code"
                  autoComplete="off"
                  placeholder="abcde-fghij"
                  required
                  autoFocus
                  aria-invalid={!!errors.recovery_code}
                />
                <InputError message={errors.recovery_code} />
              </div>
            ) : (
              <div className="grid gap-2">
                <Label htmlFor="code">Code</Label>
                <Input
                  id="code"
                  name="code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9 ]*"
                  maxLength={7}
                  placeholder="123456"
                  required
                  autoFocus
                  className="text-center font-mono text-lg tracking-[0.3em]"
                  aria-invalid={!!errors.code}
                />
                <InputError message={errors.code} />
              </div>
            )}
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              Log in
            </Button>
          </>
        )}
      </Form>
      <p className="mt-6 text-center text-sm text-muted-foreground">
        {recovery ? 'Found your phone? ' : 'Lost your phone? '}
        <button type="button" onClick={() => setRecovery(!recovery)} className="cursor-pointer text-foreground underline underline-offset-4">
          {recovery ? 'Use a code from the app' : 'Use a recovery code'}
        </button>
      </p>
    </>
  )
}

TwoFactorChallenge.layout = {
  title: 'Two-factor login',
  description: 'The six-digit code your authenticator app shows for this account.',
}
