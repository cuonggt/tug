<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
  import LoginSettings from '@/components/LoginSettings.svelte'
  import PasskeySettings from '@/components/PasskeySettings.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TwoFactorSettings from '@/components/TwoFactorSettings.svelte'
  import { Button } from '@/components/ui/button'
  import { Label } from '@/components/ui/label'
  import type { PageProps } from '@/tug/pages'
  import { form } from '@/tug/routes'

  // Security changes the user's password, which logs them out everywhere
  // else, turns two-factor logins on and off, keeps their passkeys, and
  // lists the browsers they're logged in from.
  let props: PageProps<'Settings/Security'> = $props()
</script>

<Head title="Security" />
<section class="space-y-6">
  <Heading small title="Password" description="A new one logs you out everywhere else, such as a lost laptop." />
  <Form
    action={form('user-password.update')}
    options={{ preserveScroll: true }}
    resetOnError
    resetOnSuccess
    class="space-y-6"
  >
    {#snippet children({ errors, processing })}
      <div class="grid gap-2">
        <Label for="current_password">Current password</Label>
        <PasswordInput id="current_password" name="current_password" autocomplete="current-password" required />
        <InputError message={errors.current_password} />
      </div>
      <div class="grid gap-2">
        <Label for="password">New password</Label>
        <PasswordInput id="password" name="password" autocomplete="new-password" required />
        <InputError message={errors.password} />
      </div>
      <div class="grid gap-2">
        <Label for="password_confirmation">New password again</Label>
        <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
        <InputError message={errors.password_confirmation} />
      </div>
      <Button type="submit" disabled={processing}>Change the password</Button>
    {/snippet}
  </Form>
</section>
<PasskeySettings {...props} />
<TwoFactorSettings {...props} />
<LoginSettings {...props} />
