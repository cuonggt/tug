<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
  import { useLang } from '@/lib/lang'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t, d } = useLang()

  // Dashboard is for users who've verified their email: its route in main.go
  // wraps it in usersOnly and verified, and its handler is handed the user.
  let { user }: PageProps<'Dashboard'> = $props()
  // The date is in the page's own language, and in the reader's own zone
  // once the page is in their browser.
  let since = $derived(d(user.createdAt, { dateStyle: 'long' }))
  const code = 'font-mono text-foreground'
</script>

<Head title={t('Dashboard')} />
<div class="space-y-8">
  <div class="space-y-1">
    <h1 class="text-2xl font-semibold tracking-tight">{t('Hello, :name', { name: user.name })}</h1>
    <p class="text-muted-foreground">{t("You've had an account since :date.", { date: since })}</p>
  </div>
  <div class="grid gap-4 md:grid-cols-3">
    <Card>
      <CardHeader>
        <CardTitle>{t('Your app goes here')}</CardTitle>
        <CardDescription>
          This page is <code class={code}>resources/js/pages/Dashboard.svelte</code>, and its handler is
          <code class={code}>dashboard</code> in <code class={code}>main.go</code>.
        </CardDescription>
      </CardHeader>
    </Card>
    <Card>
      <CardHeader>
        <CardTitle>{t('Add a page')}</CardTitle>
        <CardDescription>
          Declare it in Go with <code class={code}>tug.Page[Props]("Name")</code>, route it, and write
          <code class={code}>pages/Name.svelte</code>. tug dev writes the types of its props.
        </CardDescription>
      </CardHeader>
    </Card>
    <Card>
      <CardHeader>
        <CardTitle>{t('Your account')}</CardTitle>
        <CardDescription>
          {user.twoFactor ? t('Two-factor logins are on.') : t('Two-factor logins are off.')}
          {t('Your password, profile and appearance are in your settings too.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Link href={route('security.edit')} class={buttonVariants({ variant: 'outline', size: 'sm' })}>
          {t('Security settings')}
        </Link>
      </CardContent>
    </Card>
  </div>
</div>
