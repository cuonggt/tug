import { Form, router, usePage } from '@inertiajs/react'
import { Check, Copy, LoaderCircle, ShieldCheck } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useState } from 'react'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// TwoFactorSettings turns two-factor logins on and off: off, a button to
// start; starting, a QR code for the user's authenticator app and a code
// back from it; on, their recovery codes, and a button to stop. The
// handlers are in twofactor.go.
export default function TwoFactorSettings({ user, setup, recoveryCodes }: PageProps<'Settings/Security'>) {
  const { flash } = usePage()
  // The codes come once in the flash as two-factor logins are turned on,
  // and again from a partial reload that asks for them.
  const codes = flash.recoveryCodes ?? recoveryCodes

  return (
    <section className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <Heading
          small
          title="Two-factor logins"
          description="A code from an app on your phone, as well as your password, each time you log in."
        />
        <Badge variant={user.twoFactor ? 'default' : 'secondary'}>{user.twoFactor ? 'On' : 'Off'}</Badge>
      </div>

      {user.twoFactor ? (
        <>
          {codes ? (
            <RecoveryCodes codes={codes} />
          ) : (
            <div className="space-y-3">
              <p className="text-sm text-muted-foreground">
                Recovery codes log you in when your phone is lost: one code, one login.
              </p>
              <Button variant="outline" onClick={() => router.reload({ only: ['recoveryCodes'] })}>
                Show my recovery codes
              </Button>
            </div>
          )}
          <Form action={route('two-factor.disable')} method="delete" options={{ preserveScroll: true }}>
            {({ processing }) => (
              <Button type="submit" variant="destructive" disabled={processing}>
                Turn two-factor logins off
              </Button>
            )}
          </Form>
        </>
      ) : setup ? (
        <div className="space-y-6">
          <ol className="list-decimal space-y-2 pl-5 text-sm text-muted-foreground">
            <li>Scan this with an authenticator app, such as 1Password, Google Authenticator or Authy.</li>
            <li>Type the code it shows, to be sure it's set up.</li>
          </ol>
          <div className="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
            <div className="rounded-lg border bg-white p-3">
              <QRCodeSVG value={setup.url} size={160} marginSize={0} title="The QR code for your authenticator app" />
            </div>
            <div className="space-y-1 text-sm">
              <p className="text-muted-foreground">Can't scan it? Type this key into the app instead:</p>
              <p className="font-mono text-base tracking-wider break-all select-all">
                {setup.secret.match(/.{1,4}/g)?.join(' ')}
              </p>
            </div>
          </div>
          <Form action={route('two-factor.confirm')} method="post" resetOnError options={{ preserveScroll: true }} className="space-y-4">
            {({ errors, processing }) => (
              <>
                <div className="grid max-w-48 gap-2">
                  <Label htmlFor="code">The code from the app</Label>
                  <Input
                    id="code"
                    name="code"
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={7}
                    placeholder="123456"
                    required
                    autoFocus
                    className="font-mono tracking-widest"
                    aria-invalid={!!errors.code}
                  />
                </div>
                <InputError message={errors.code} />
                <div className="flex gap-2">
                  <Button type="submit" disabled={processing}>
                    {processing && <LoaderCircle className="animate-spin" />}
                    Turn on
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    onClick={() => router.delete(route('two-factor.disable'), { preserveScroll: true })}
                  >
                    Cancel
                  </Button>
                </div>
              </>
            )}
          </Form>
        </div>
      ) : (
        <Form action={route('two-factor.enable')} method="post" options={{ preserveScroll: true }}>
          {({ processing }) => (
            <Button type="submit" disabled={processing}>
              <ShieldCheck />
              Turn two-factor logins on
            </Button>
          )}
        </Form>
      )}
    </section>
  )
}

// RecoveryCodes lists the user's recovery codes, to copy somewhere safe,
// and makes new ones.
function RecoveryCodes({ codes }: { codes: string[] }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    await navigator.clipboard.writeText(codes.join('\n'))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Keep these somewhere safe, such as a password manager. Each logs you in once, when your phone is lost.
      </p>
      {codes.length > 0 ? (
        <ul className="grid grid-cols-2 gap-x-6 gap-y-1 rounded-lg bg-muted p-4 font-mono text-sm">
          {codes.map((code) => (
            <li key={code}>{code}</li>
          ))}
        </ul>
      ) : (
        <p className="rounded-lg bg-muted p-4 text-sm">You've used them all: make new ones.</p>
      )}
      <div className="flex flex-wrap gap-2">
        {codes.length > 0 && (
          <Button type="button" variant="outline" onClick={copy}>
            {copied ? <Check /> : <Copy />}
            {copied ? 'Copied' : 'Copy'}
          </Button>
        )}
        <Form action={route('two-factor.recovery-codes')} method="post" options={{ preserveScroll: true }}>
          {({ processing }) => (
            <Button type="submit" variant="outline" disabled={processing}>
              Make new codes
            </Button>
          )}
        </Form>
      </div>
    </div>
  )
}
