<script module lang="ts">
  export const layout = { title: 'Log in', description: 'Welcome back: your passkey, or your email and password, please.' }
</script>

<script lang="ts">
  import { Form, page, router } from '@inertiajs/svelte'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { Checkbox } from '@/components/ui/checkbox'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { autofillWorks, dismissed, getPasskey, type Json, optionsFrom, passkeysWork } from '@/lib/passkeys'
  import { route } from '@/tug/routes'

  // Login logs in with an email and password, or with a passkey, which the
  // browser offers in the email field's autofill too. passkeyLogin in
  // passkeys.go takes the passkey's.
  let remember = $state(false)
  let works = $state(false)
  let failure = $state<string>()
  let autofill: AbortController | undefined

  const logIn = (credential: Json) => router.post(route('login.passkey'), { credential, remember })

  // The browser offers the site's passkeys as the email field is focused,
  // for as long as the page is open.
  onMount(() => {
    works = passkeysWork()
    const abort = new AbortController()
    autofill = abort
    ;(async () => {
      if (!(await autofillWorks())) return
      const options = await optionsFrom(route('login.passkey.options'))
      if (options) logIn(await getPasskey(options, abort.signal))
    })().catch((err) => {
      if (!dismissed(err)) failure = "Your browser didn't use a passkey: try again, or use your password."
    })
    return () => abort.abort()
  })

  async function withPasskey() {
    autofill?.abort() // a browser asks for one passkey at a time
    failure = undefined
    try {
      const options = await optionsFrom(route('login.passkey.options'))
      if (options) logIn(await getPasskey(options))
    } catch (err) {
      if (!dismissed(err)) failure = "Your browser didn't use a passkey: try again, or use your password."
    }
  }
</script>

<Head title="Log in" />
<Form action={route('login.store')} method="post" resetOnError={['password']} class="flex flex-col gap-6">
  {#snippet children({ errors, processing })}
    <div class="grid gap-2">
      <Label for="email">Email</Label>
      <Input
        id="email"
        name="email"
        type="email"
        autocomplete="username webauthn"
        placeholder="you@example.com"
        required
        autofocus
        aria-invalid={!!errors.email}
      />
      <InputError message={errors.email} />
    </div>
    <div class="grid gap-2">
      <div class="flex items-center">
        <Label for="password">Password</Label>
        <TextLink href={route('password.request')} class="ml-auto text-sm">Forgotten it?</TextLink>
      </div>
      <PasswordInput id="password" name="password" autocomplete="current-password" required />
      <InputError message={errors.password} />
    </div>
    <div class="flex items-center gap-3">
      <Checkbox id="remember" name="remember" bind:checked={remember} />
      <Label for="remember" class="font-normal">Remember me for a month</Label>
    </div>
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      Log in
    </Button>
  {/snippet}
</Form>
{#if works}
  <div class="mt-4 grid gap-2">
    <Button type="button" variant="outline" class="w-full" onclick={withPasskey}>
      <KeyRound />
      Log in with a passkey
    </Button>
    <InputError message={page.props.errors.passkey ?? failure} />
  </div>
{/if}
<p class="mt-6 text-center text-sm text-muted-foreground">
  No account yet? <TextLink href={route('register')}>Register</TextLink>
</p>
