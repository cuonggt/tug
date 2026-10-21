import { Form, Head } from '@inertiajs/react'
import { LoaderCircle } from 'lucide-react'
import InputError from '@/components/input-error'
import TextLink from '@/components/text-link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useLang } from '@/lib/lang'
import { text } from '@/tug/lang'
import { form, route, type Inputs } from '@/tug/routes'

// ForgotPassword mails a link that sets a new password. In development,
// without MAIL_HOST in .env, the mail is kept in the app's mailbox, at
// /_tug/mail, which tug dev's terminal links to.
export default function ForgotPassword() {
  const { t } = useLang()

  return (
    <>
      <Head title={t('Forgotten password')} />
      <Form<Inputs['password.email']> action={form('password.email')} resetOnSuccess className="flex flex-col gap-6">
        {({ errors, processing }) => (
          <>
            <div className="grid gap-2">
              <Label htmlFor="email">{t('Email')}</Label>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                required
                autoFocus
                aria-invalid={!!errors.email}
              />
              <InputError message={errors.email} />
            </div>
            <Button type="submit" className="w-full" disabled={processing}>
              {processing && <LoaderCircle className="animate-spin" />}
              {t('Mail me a link')}
            </Button>
          </>
        )}
      </Form>
      <p className="mt-6 text-center text-sm text-muted-foreground">
        {t('Remembered it?')} <TextLink href={route('login')}>{t('Log in')}</TextLink>
      </p>
    </>
  )
}

ForgotPassword.layout = {
  title: text('Forgotten your password, or lost your passkey?'),
  description: text("Say the email of your account, and we'll mail you a link to choose a new password."),
}
