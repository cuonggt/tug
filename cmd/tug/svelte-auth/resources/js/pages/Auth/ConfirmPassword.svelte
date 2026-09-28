<script module lang="ts">
  export const layout = {
    title: 'Confirm your password',
    description: "You're on your way to your account's settings, which ask for it once in a while.",
  }
</script>

<script lang="ts">
  import { Form, page, router } from '@inertiajs/svelte'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import { Button } from '@/components/ui/button'
  import { Label } from '@/components/ui/label'
  import { dismissed, getPasskey, optionsFrom, passkeysWork } from '@/lib/passkeys'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // ConfirmPassword asks for the password again before the settings that
  // could hand the account to someone else: passwordConfirmed in auth.go
  // sends users here when they last typed it over three hours ago. One of
  // their passkeys says it's them as well.
  let { passkeys }: PageProps<'Auth/ConfirmPassword'> = $props()
  let works = $state(false)
  let failure = $state<string>()
  onMount(() => {
    works = passkeysWork()
  })

  async function withPasskey() {
    failure = undefined
    try {
      const options = await optionsFrom(route('password.confirm.passkey.options'))
      if (options) router.post(route('password.confirm.passkey'), { credential: await getPasskey(options) })
    } catch (err) {
      if (!dismissed(err)) failure = "Your browser didn't use a passkey: try again, or type your password."
    }
  }
</script>

<Head title="Confirm your password" />
<Form action={route('password.confirm.store')} method="post" resetOnError class="flex flex-col gap-6">
  {#snippet children({ errors, processing })}
    <div class="grid gap-2">
      <Label for="password">Password</Label>
      <PasswordInput id="password" name="password" autocomplete="current-password" required autofocus />
      <InputError message={errors.password} />
    </div>
    <Button type="submit" class="w-full" disabled={processing}>
      {#if processing}
        <LoaderCircle class="animate-spin" />
      {/if}
      Confirm
    </Button>
  {/snippet}
</Form>
{#if passkeys && works}
  <div class="mt-4 grid gap-2">
    <Button type="button" variant="outline" class="w-full" onclick={withPasskey}>
      <KeyRound />
      Use a passkey
    </Button>
    <InputError message={page.props.errors.passkey ?? failure} />
  </div>
{/if}
