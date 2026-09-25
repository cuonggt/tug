import { Form, Head } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { route } from '@/tug/routes'

// ConfirmPassword asks for the password again before the settings that
// could hand the account to someone else: passwordConfirmed in auth.go
// sends users here when they last typed it over three hours ago.
export default function ConfirmPassword() {
  return (
    <>
      <Head title="Confirm your password" />
      <Form action={route('password.confirm.store')} method="post" resetOnError className="flex flex-col gap-6">
        {({ errors, processing }) => (
          <>
            <div className="grid gap-2">
              <Label htmlFor="password">Password</Label>
              <PasswordInput id="password" name="password" autoComplete="current-password" required autoFocus />
              <InputError message={errors.password} />
            </div>
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              Confirm
            </Button>
          </>
        )}
      </Form>
    </>
  )
}

ConfirmPassword.layout = {
  title: 'Confirm your password',
  description: "You're on your way to your account's settings, which ask for it once in a while.",
}
