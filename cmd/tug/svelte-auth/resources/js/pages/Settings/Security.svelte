<script lang="ts">
  import { createForm, router } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
  import LoginSettings from '@/components/LoginSettings.svelte'
  import PasskeySettings from '@/components/PasskeySettings.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TwoFactorSettings from '@/components/TwoFactorSettings.svelte'
  import { Button } from '@/components/ui/button'
  import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
  import type { PageProps } from '@/tug/pages'
  import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

  // Security changes the user's password, which logs them out everywhere
  // else, or sets one, for an account made with a passkey, or removes it,
  // for one with a passkey to log in with; turns two-factor logins on and
  // off, keeps their passkeys, and lists the browsers they're logged in
  // from.
  let props: PageProps<'Settings/Security'> = $props()

  // The form, typed by its route's input, as tug gen writes it from the
  // Go: its errors are the fields it sends.
  const PasswordForm = createForm<Inputs['user-password.update']>()
</script>

<Head title={t('Security')} />
<section class="space-y-6">
  <Heading
    small
    title={t('Password')}
    description={props.user.password
      ? t('A new one logs you out everywhere else, such as a lost laptop.') : t('Your account logs in with its passkeys alone. A password logs in too, and logs you out everywhere else.')}
  />
  <PasswordForm
    action={form('user-password.update')}
    options={{ preserveScroll: true }}
    resetOnError
    resetOnSuccess
    class="space-y-6"
  >
    {#snippet children({ errors, processing })}
      {#if props.user.password}
        <div class="grid gap-2">
          <Label for="current_password">{t('Current password')}</Label>
          <PasswordInput id="current_password" name="current_password" autocomplete="current-password" required />
          <InputError message={errors.current_password} />
        </div>
      {/if}
      <div class="grid gap-2">
        <Label for="password">{t('New password')}</Label>
        <PasswordInput id="password" name="password" autocomplete="new-password" required />
        <InputError message={errors.password} />
      </div>
      <div class="grid gap-2">
        <Label for="password_confirmation">{t('New password again')}</Label>
        <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
        <InputError message={errors.password_confirmation} />
      </div>
      <Button type="submit" disabled={processing}>{props.user.password ? t('Change the password') : t('Set a password')}</Button>
    {/snippet}
  </PasswordForm>
  {#if props.user.password && props.passkeys.length > 0}
    <div class="space-y-2">
      <p class="text-sm text-muted-foreground">{t('Your passkeys log you in without it.')}</p>
      <Button variant="outline" onclick={() => router.delete(route('user-password.destroy'), { preserveScroll: true })}>
        {t('Remove the password')}
      </Button>
    </div>
  {/if}
</section>
<PasskeySettings {...props} />
<TwoFactorSettings {...props} />
<LoginSettings {...props} />
