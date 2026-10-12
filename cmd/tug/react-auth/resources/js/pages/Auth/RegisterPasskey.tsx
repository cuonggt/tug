import { Head, router, usePage } from '@inertiajs/react'
import { KeyRound, LoaderCircle } from 'lucide-react'
import { useEffect, useState } from 'react'
import InputError from '@/components/input-error'
import { Button } from '@/components/ui/button'
import { createPasskey, dismissed, optionsFrom, passkeysWork, Refused } from '@/lib/passkeys'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// RegisterPasskey is the page the link mailed to register with a passkey
// opens: the browser makes the passkey, and registerPasskey in passkeys.go
// makes the account with it, its email verified by the link, with no
// password.
export default function RegisterPasskey({ token, email, name }: PageProps<'Auth/RegisterPasskey'>) {
  const { errors } = usePage().props
  const [making, setMaking] = useState(false)
  const [failure, setFailure] = useState<string>()
  // Known once the page is in the browser, so the server's HTML is the same.
  const [works, setWorks] = useState<boolean>()
  useEffect(() => setWorks(passkeysWork()), [])

  const make = async () => {
    setMaking(true)
    setFailure(undefined)
    try {
      const query = new URLSearchParams({ email, name })
      const options = await optionsFrom(`${route('register.passkey.options', { token })}?${query}`)
      if (!options) return
      const credential = await createPasskey(options)
      router.post(route('register.passkey.store', { token }), { email, name, credential }, { onFinish: () => setMaking(false) })
    } catch (err) {
      setMaking(false)
      if (err instanceof Refused) setFailure(err.message)
      else if (!dismissed(err)) setFailure("Your browser didn't make the passkey: try again.")
    }
  }

  return (
    <>
      <Head title="Make your passkey" />
      <div className="flex flex-col gap-6">
        <p className="text-sm">
          Your account is <strong>{name}</strong>, at <strong>{email}</strong>, with no password: your browser asks where to
          keep the passkey.
        </p>
        <div className="grid gap-2">
          <Button type="button" className="w-full" onClick={make} disabled={making || !works}>
            {making ? <LoaderCircle className="animate-spin" /> : <KeyRound />}
            Make your passkey
          </Button>
          <InputError message={errors.passkey ?? failure} />
          {works === false && (
            <p className="text-sm text-muted-foreground">This browser doesn't make passkeys: open the link in one that does.</p>
          )}
        </div>
      </div>
    </>
  )
}

RegisterPasskey.layout = {
  title: 'Make your passkey',
  description: 'It logs you in with your phone, laptop or password manager, and its PIN, fingerprint or face.',
}
