<script module lang="ts">
  export const layout = {
    title: 'Make an account',
    description: 'Your name, your email, and a password of 8 characters or more, or a passkey.',
  }
</script>

<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import LockKeyhole from '@lucide/svelte/icons/lock-keyhole'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { passkeysWork } from '@/lib/passkeys'
  import { form, route } from '@/tug/routes'

  // Register checks each field with the server as it's left, as BindValid in
  // auth.go answers: a taken email shows before the form is sent. Where the
  // browser makes passkeys, the account can have one in place of a password:
  // the form sends the name and the email alone, and registerWithPasskey in
  // passkeys.go mails a link, whose page makes the passkey and the account. Its Form
  // takes no type, as the other pages' do with createForm: the typed one's
  // type leaves out the methods bind:this gives, validator among them,
  // which the page calls.
  let registration: Form

  // Known once the page is in the browser, so the server's HTML is the same.
  let works = $state(false)
  let withPasskey = $state(false)

  // A field's check can be waiting out its 300ms as the form is sent, and
  // Inertia's Svelte Form throws when it runs: after registering has logged
  // the browser in, guestsOnly redirects it, which isn't Precognition's
  // answer, and once the next page has taken this one's place, it reads the
  // form that's gone (@inertiajs/svelte 3.7.1; React's Form sets its
  // timeout afresh each time it renders, as when it's sent, and checks for
  // the form). So it's dropped as the form is sent, and as the page goes:
  // setting the validator's timeout makes it afresh, with nothing waiting.
  onMount(() => {
    works = passkeysWork()
    const validator = registration.validator()
    return () => validator.setTimeout(300)
  })
</script>

<Head title="Register" />
<Form
  bind:this={registration}
  action={withPasskey ? form('register.passkey') : form('register.store')}
  resetOnError={['password', 'password_confirmation']}
  validationTimeout={300}
  onStart={() => registration.validator().setTimeout(300)}
  class="flex flex-col gap-6"
>
  {#snippet children({ errors, processing, validate, invalid })}
    <div class="grid gap-2">
      <Label for="name">Name</Label>
      <Input
        id="name"
        name="name"
        autocomplete="name"
        required
        autofocus
        aria-invalid={invalid('name')}
        onblur={() => validate('name')}
      />
      <InputError message={errors.name} />
    </div>
    <div class="grid gap-2">
      <Label for="email">Email</Label>
      <Input
        id="email"
        name="email"
        type="email"
        autocomplete="email"
        placeholder="you@example.com"
        required
        aria-invalid={invalid('email')}
        onblur={() => validate('email')}
      />
      <InputError message={errors.email} />
    </div>
    {#if withPasskey}
      <p class="text-sm text-muted-foreground">
        We'll mail you a link: it makes your passkey, and your account with it, with no password.
      </p>
    {:else}
      <div class="grid gap-2">
        <Label for="password">Password</Label>
        <PasswordInput
          id="password"
          name="password"
          autocomplete="new-password"
          required
          aria-invalid={invalid('password')}
          onblur={() => validate('password')}
        />
        <InputError message={errors.password} />
      </div>
      <div class="grid gap-2">
        <Label for="password_confirmation">Password again</Label>
        <PasswordInput
          id="password_confirmation"
          name="password_confirmation"
          autocomplete="new-password"
          required
          aria-invalid={invalid('password_confirmation')}
          onblur={() => validate('password_confirmation')}
        />
        <InputError message={errors.password_confirmation} />
      </div>
    {/if}
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      {withPasskey ? 'Mail me a link' : 'Register'}
    </Button>
  {/snippet}
</Form>
{#if works}
  <Button type="button" variant="outline" class="mt-4 w-full" onclick={() => (withPasskey = !withPasskey)}>
    {#if withPasskey}
      <LockKeyhole />
      Use a password instead
    {:else}
      <KeyRound />
      Use a passkey instead
    {/if}
  </Button>
{/if}
<p class="mt-6 text-center text-sm text-muted-foreground">
  Have an account? <TextLink href={route('login')}>Log in</TextLink>
</p>
