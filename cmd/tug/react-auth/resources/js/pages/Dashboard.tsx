import { Head, Link } from '@inertiajs/react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useLang } from '@/lib/lang'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Dashboard is for users who've verified their email: its route in main.go
// wraps it in usersOnly and verified, and its handler is handed the user.
export default function Dashboard({ user }: PageProps<'Dashboard'>) {
  const { t, d } = useLang()

  // The date is in the page's own language, and in the reader's own zone
  // once the page is in their browser.
  const since = d(user.createdAt, { dateStyle: 'long' })
  const code = 'font-mono text-foreground'
  return (
    <>
      <Head title={t('Dashboard')} />
      <div className="space-y-8">
        <div className="space-y-1">
          <h1 className="text-2xl font-semibold tracking-tight">{t('Hello, :name', { name: user.name })}</h1>
          <p className="text-muted-foreground">{t("You've had an account since :date.", { date: since })}</p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle>{t('Your app goes here')}</CardTitle>
              <CardDescription>
                This page is <code className={code}>resources/js/pages/Dashboard.tsx</code>, and its handler is{' '}
                <code className={code}>dashboard</code> in <code className={code}>main.go</code>.
              </CardDescription>
            </CardHeader>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('Add a page')}</CardTitle>
              <CardDescription>
                Declare it in Go with <code className={code}>tug.Page[Props]("Name")</code>, route it, and write{' '}
                <code className={code}>pages/Name.tsx</code>. tug dev writes the types of its props.
              </CardDescription>
            </CardHeader>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t('Your account')}</CardTitle>
              <CardDescription>
                {user.twoFactor ? t('Two-factor logins are on.') : t('Two-factor logins are off.')}{' '}
                {t('Your password, profile and appearance are in your settings too.')}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Button variant="outline" size="sm" asChild>
                <Link href={route('security.edit')}>{t('Security settings')}</Link>
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    </>
  )
}
