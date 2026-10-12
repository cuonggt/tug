<script module lang="ts">
  export const layout = {
    title: 'Forgotten your password, or lost your passkey?',
    description: "Say the email of your account, and we'll mail you a link to choose a new password.",
  }
</script>

<script lang="ts">
  import { createForm } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { form, route, type Inputs } from '@/tug/routes'

  // ForgotPassword mails a link that sets a new password. In development,
  // without MAIL_HOST in .env, the mail is kept in the app's mailbox, at
  // /_tug/mail, which tug dev's terminal links to.

  // The form, typed by its route's input, as tug gen writes it from the
  // Go: its errors are the fields it sends.
  const ResetLinkForm = createForm<Inputs['password.email']>()
</script>

<Head title="Forgotten password" />
<ResetLinkForm action={form('password.email')} resetOnSuccess class="flex flex-col gap-6">
  {#snippet children({ errors, processing })}
    <div class="grid gap-2">
      <Label for="email">Email</Label>
      <Input
        id="email"
        name="email"
        type="email"
        autocomplete="email"
        placeholder="you@example.com"
        required
        autofocus
        aria-invalid={!!errors.email}
      />
      <InputError message={errors.email} />
    </div>
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      Mail me a link
    </Button>
  {/snippet}
</ResetLinkForm>
<p class="mt-6 text-center text-sm text-muted-foreground">
  Remembered it? <TextLink href={route('login')}>Log in</TextLink>
</p>
