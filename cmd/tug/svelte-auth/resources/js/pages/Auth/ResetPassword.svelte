<script module lang="ts">
  export const layout = { title: 'Choose a new password', description: 'It logs you out everywhere you were logged in.' }
</script>

<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // ResetPassword is where the link in a reset mail leads: its token, and
  // the email it went to, come in the props.
  let { token, email }: PageProps<'Auth/ResetPassword'> = $props()
</script>

<Head title="Choose a new password" />
<Form
  action={route('password.store')}
  method="post"
  resetOnError={['password', 'password_confirmation']}
  class="flex flex-col gap-6"
>
  {#snippet children({ errors, processing })}
    <input type="hidden" name="token" value={token} />
    <div class="grid gap-2">
      <Label for="email">Email</Label>
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
      <Label for="password">New password</Label>
      <PasswordInput id="password" name="password" autocomplete="new-password" required autofocus />
      <InputError message={errors.password} />
    </div>
    <div class="grid gap-2">
      <Label for="password_confirmation">New password again</Label>
      <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
      <InputError message={errors.password_confirmation} />
    </div>
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      Set the password
    </Button>
  {/snippet}
</Form>
<p class="mt-6 text-center text-sm text-muted-foreground">
  Has the link stopped working? <TextLink href={route('password.request')}>Ask for another</TextLink>
</p>
