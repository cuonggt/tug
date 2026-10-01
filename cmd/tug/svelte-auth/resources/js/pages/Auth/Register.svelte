<script module lang="ts">
  export const layout = { title: 'Make an account', description: 'Your name, your email, and a password of 8 characters or more.' }
</script>

<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { form, route } from '@/tug/routes'

  // Register checks each field with the server as it's left, as BindValid in
  // auth.go answers: a taken email shows before the form is sent.
  let registration: Form

  // A field's check can be waiting out its 300ms as the form is sent, and
  // Inertia's Svelte Form throws when it runs: after registering has logged
  // the browser in, guestsOnly redirects it, which isn't Precognition's
  // answer, and once the next page has taken this one's place, it reads the
  // form that's gone (@inertiajs/svelte 3.7.1; React's Form sets its
  // timeout afresh each time it renders, as when it's sent, and checks for
  // the form). So it's dropped as the form is sent, and as the page goes:
  // setting the validator's timeout makes it afresh, with nothing waiting.
  onMount(() => {
    const validator = registration.validator()
    return () => validator.setTimeout(300)
  })
</script>

<Head title="Register" />
<Form
  bind:this={registration}
  action={form('register.store')}
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
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      Register
    </Button>
  {/snippet}
</Form>
<p class="mt-6 text-center text-sm text-muted-foreground">
  Have an account? <TextLink href={route('login')}>Log in</TextLink>
</p>
