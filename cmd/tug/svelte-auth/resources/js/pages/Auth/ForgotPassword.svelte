<script module lang="ts">
  export const layout = {
    title: 'Forgotten your password?',
    description: "Say the email of your account, and we'll mail you a link to choose a new one.",
  }
</script>

<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { route } from '@/tug/routes'

  // ForgotPassword mails a link that sets a new password. In development,
  // without MAIL_HOST in .env, the mail is written to tug dev's terminal.
</script>

<Head title="Forgotten password" />
<Form action={route('password.email')} method="post" resetOnSuccess class="flex flex-col gap-6">
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
</Form>
<p class="mt-6 text-center text-sm text-muted-foreground">
  Remembered it? <TextLink href={route('login')}>Log in</TextLink>
</p>
