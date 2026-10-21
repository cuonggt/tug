import { Form, router, usePage } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import { useRef, useState } from 'react'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import PasswordInput from '@/components/password-input'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { useLang } from '@/lib/lang'
import { dismissed, getPasskey, optionsFrom } from '@/lib/passkeys'
import { form, route, type Inputs } from '@/tug/routes'

// DeleteAccount deletes the user's account, after asking for the password
// once more, or for an account with none, a passkey: it can't be undone.
export default function DeleteAccount() {
  const { t } = useLang()

  const { auth, errors } = usePage().props
  const password = useRef<HTMLInputElement>(null)
  const [deleting, setDeleting] = useState(false)
  const [failure, setFailure] = useState<string>()

  const withPasskey = async () => {
    setDeleting(true)
    setFailure(undefined)
    try {
      const options = await optionsFrom(route('password.confirm.passkey.options'))
      if (!options) return
      const credential = await getPasskey(options)
      router.delete(route('profile.destroy'), { data: { credential }, preserveScroll: true, onFinish: () => setDeleting(false) })
    } catch (err) {
      setDeleting(false)
      if (!dismissed(err)) setFailure(t("Your browser didn't use a passkey: try again."))
    }
  }

  return (
    <section className="space-y-6">
      <Heading small title={t('Delete your account')} description={t('Your account, and everything in it, for good.')} />
      <div className="space-y-4 rounded-lg border border-destructive/30 bg-destructive/5 p-4">
        <p className="text-sm text-destructive">{t("Once it's gone, it's gone: there's no undoing it.")}</p>
        <Dialog>
          <DialogTrigger asChild>
            <Button variant="destructive">{t('Delete my account')}</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogTitle>{t('Delete your account?')}</DialogTitle>
            {auth.user?.password ? (
              <>
                <DialogDescription>
                  {t("Everything in it goes too, and you're logged out everywhere. Type your password to say you mean it.")}
                </DialogDescription>
                <Form<Inputs['profile.destroy']>
                  action={form('profile.destroy')}
                  options={{ preserveScroll: true }}
                  onError={() => password.current?.focus()}
                  resetOnError
                  className="space-y-6"
                >
                  {({ errors, processing, resetAndClearErrors }) => (
                    <>
                      <div className="grid gap-2">
                        <Label htmlFor="delete-password" className="sr-only">
                          {t('Password')}
                        </Label>
                        <PasswordInput
                          id="delete-password"
                          name="password"
                          ref={password}
                          autoComplete="current-password"
                          placeholder={t('Password')}
                        />
                        <InputError message={errors.password} />
                      </div>
                      <DialogFooter className="gap-2">
                        <DialogClose asChild>
                          <Button type="button" variant="secondary" onClick={() => resetAndClearErrors()}>
                            {t('Keep it')}
                          </Button>
                        </DialogClose>
                        <Button type="submit" variant="destructive" disabled={processing}>
                          {t('Delete my account')}
                        </Button>
                      </DialogFooter>
                    </>
                  )}
                </Form>
              </>
            ) : (
              <>
                <DialogDescription>
                  {t("Everything in it goes too, and you're logged out everywhere. Use your passkey to say you mean it.")}
                </DialogDescription>
                <InputError message={errors.passkey ?? failure} />
                <DialogFooter className="gap-2">
                  <DialogClose asChild>
                    <Button type="button" variant="secondary">
                      {t('Keep it')}
                    </Button>
                  </DialogClose>
                  <Button type="button" variant="destructive" onClick={withPasskey} disabled={deleting}>
                    {deleting && <LoaderCircle className="animate-spin" />}
                    {t('Delete my account')}
                  </Button>
                </DialogFooter>
              </>
            )}
          </DialogContent>
        </Dialog>
      </div>
    </section>
  )
}
