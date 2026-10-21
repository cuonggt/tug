<script module lang="ts">
  export const layout = { title: text('Choose a new password'), description: text('It logs you out everywhere you were logged in.') }
</script>

<script lang="ts">
  import { createForm } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
  import { text } from '@/tug/lang'
  import type { PageProps } from '@/tug/pages'
  import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

  // ResetPassword is where the link in a reset mail leads: its token, and
  // the email it went to, come in the props.
  let { token, email }: PageProps<'Auth/ResetPassword'> = $props()

  // The form, typed by its route's input, as tug gen writes it from the
  // Go: its errors are the fields it sends.
  const ResetForm = createForm<Inputs['password.store']>()
</script>

<Head title={t('Choose a new password')} />
<ResetForm
  action={form('password.store')}
  resetOnError={['password', 'password_confirmation']}
  class="flex flex-col gap-6"
>
  {#snippet children({ errors, processing })}
    <input type="hidden" name="token" value={token} />
    <div class="grid gap-2">
      <Label for="email">{t('Email')}</Label>
      <Input
        id="email"
        name="email"
        type="email"
        autocomplete="username"
        value={email}
        readonly
        aria-invalid={!!errors.email}
      />
      <InputError message={errors.email} />
    </div>
    <div class="grid gap-2">
      <Label for="password">{t('New password')}</Label>
      <PasswordInput id="password" name="password" autocomplete="new-password" required autofocus />
      <InputError message={errors.password} />
    </div>
    <div class="grid gap-2">
      <Label for="password_confirmation">{t('New password again')}</Label>
      <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
      <InputError message={errors.password_confirmation} />
    </div>
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      {t('Set the password')}
    </Button>
  {/snippet}
</ResetForm>
<p class="mt-6 text-center text-sm text-muted-foreground">
  {t('Has the link stopped working?')} <TextLink href={route('password.request')}>{t('Ask for another')}</TextLink>
</p>
