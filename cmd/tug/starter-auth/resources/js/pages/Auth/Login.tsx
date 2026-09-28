import { Form, Head, router, usePage } from '@inertiajs/react'
import { KeyRound, LoaderCircle } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { autofillWorks, dismissed, getPasskey, type Json, optionsFrom, passkeysWork } from '@/lib/passkeys'
import { route } from '@/tug/routes'

// Login logs in with an email and password, or with a passkey, which the
// browser offers in the email field's autofill too. passkeyLogin in
// passkeys.go takes the passkey's.
export default function Login() {
  const { errors } = usePage().props
  const [remember, setRemember] = useState(false)
  const remembered = useRef(remember)
  remembered.current = remember
  const [works, setWorks] = useState(false)
  const [failure, setFailure] = useState<string>()
  const autofill = useRef<AbortController>(null)

  const logIn = (credential: Json) =>
    router.post(route('login.passkey'), { credential, remember: remembered.current })

  // The browser offers the site's passkeys as the email field is focused,
  // for as long as the page is open.
  useEffect(() => {
    setWorks(passkeysWork())
    const abort = new AbortController()
    autofill.current = abort
    ;(async () => {
      if (!(await autofillWorks())) return
      const options = await optionsFrom(route('login.passkey.options'))
      if (options) logIn(await getPasskey(options, abort.signal))
    })().catch((err) => {
      if (!dismissed(err)) setFailure("Your browser didn't use a passkey: try again, or use your password.")
    })
    return () => abort.abort()
  }, [])

  const withPasskey = async () => {
    autofill.current?.abort() // a browser asks for one passkey at a time
    setFailure(undefined)
    try {
      const options = await optionsFrom(route('login.passkey.options'))
      if (options) logIn(await getPasskey(options))
    } catch (err) {
      if (!dismissed(err)) setFailure("Your browser didn't use a passkey: try again, or use your password.")
    }
  }

  return (
    <>
      <Head title="Log in" />
      <Form
        action={route('login.store')}
        method="post"
        resetOnError={['password']}
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
                autoComplete="username webauthn"
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
              <Checkbox
                id="remember"
                name="remember"
                checked={remember}
                onCheckedChange={(checked) => setRemember(checked === true)}
              />
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
      {works && (
        <div className="mt-4 grid gap-2">
          <Button type="button" variant="outline" className="w-full" onClick={withPasskey}>
            <KeyRound />
            Log in with a passkey
          </Button>
          <InputError message={errors.passkey ?? failure} />
        </div>
      )}
      <p className="mt-6 text-center text-sm text-muted-foreground">
        No account yet? <TextLink href={route('register')}>Register</TextLink>
      </p>
    </>
  )
}

Login.layout = { title: 'Log in', description: 'Welcome back: your passkey, or your email and password, please.' }
