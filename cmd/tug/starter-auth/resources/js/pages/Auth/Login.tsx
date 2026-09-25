import { Form, Head } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { route } from '@/tug/routes'

export default function Login() {
  return (
    <>
      <Head title="Log in" />
      <Form
        action={route('login.store')}
        method="post"
        resetOnError={['password']}
        // A ticked checkbox sends "on"; LoginInput.Remember in auth.go is a
        // bool, which JSON has as true or false.
        transform={(data) => ({ ...data, remember: data.remember === 'on' })}
        className="flex flex-col gap-6"
      >
        {({ errors, processing }) => (
          <>
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="username"
                placeholder="you@example.com"
                required
                autoFocus
                aria-invalid={!!errors.email}
              />
              <InputError message={errors.email} />
            </div>
            <div className="grid gap-2">
              <div className="flex items-center">
                <Label htmlFor="password">Password</Label>
                <TextLink href={route('password.request')} className="ml-auto text-sm">
                  Forgotten it?
                </TextLink>
              </div>
              <PasswordInput id="password" name="password" autoComplete="current-password" required />
              <InputError message={errors.password} />
            </div>
            <div className="flex items-center gap-3">
              <Checkbox id="remember" name="remember" />
              <Label htmlFor="remember" className="font-normal">
                Remember me for a month
              </Label>
            </div>
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              Log in
            </Button>
          </>
        )}
      </Form>
      <p className="mt-6 text-center text-sm text-muted-foreground">
        No account yet? <TextLink href={route('register')}>Register</TextLink>
      </p>
    </>
  )
}

Login.layout = { title: 'Log in', description: 'Welcome back: your email and password, please.' }
