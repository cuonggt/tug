<script module lang="ts">
  export const layout = {
    title: text('Make your passkey'),
    description: text('It logs you in with your phone, laptop or password manager, and its PIN, fingerprint or face.'),
  }
</script>

<script lang="ts">
  import { page, router } from '@inertiajs/svelte'
  import KeyRound from '@lucide/svelte/icons/key-round'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import { Button } from '@/components/ui/button'
  import { useLang } from '@/lib/lang'
  import { createPasskey, dismissed, optionsFrom, passkeysWork, Refused } from '@/lib/passkeys'
  import { text } from '@/tug/lang'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

  // RegisterPasskey is the page the link mailed to register with a passkey
  // opens: the browser makes the passkey, and registerPasskey in passkeys.go
  // makes the account with it, its email verified by the link, with no
  // password.
  let { token, email, name }: PageProps<'Auth/RegisterPasskey'> = $props()
  let making = $state(false)
  let failure = $state<string>()
  // Known once the page is in the browser, so the server's HTML is the same.
  let works = $state<boolean>()
  onMount(() => {
    works = passkeysWork()
  })

  async function make() {
    making = true
    failure = undefined
    try {
      const query = new URLSearchParams({ email, name })
      const options = await optionsFrom(`${route('register.passkey.options', { token })}?${query}`)
      if (!options) return
      const credential = await createPasskey(options)
      router.post(route('register.passkey.store', { token }), { email, name, credential }, { onFinish: () => (making = false) })
    } catch (err) {
      making = false
      if (err instanceof Refused) failure = err.message
      else if (!dismissed(err)) failure = t("Your browser didn't make the passkey: try again.")
    }
  }
</script>

<Head title={t('Make your passkey')} />
<div class="flex flex-col gap-6">
  <p class="text-sm">
    {t('Your account is')} <strong>{name}</strong>{t(', at')} <strong>{email}</strong>{t(', with no password: your browser asks where to keep the passkey.')}
  </p>
  <div class="grid gap-2">
    <Button type="button" class="w-full" disabled={making || !works} onclick={make}>
      {#if making}
        <LoaderCircle class="animate-spin" />
      {:else}
        <KeyRound />
      {/if}
      {t('Make your passkey')}
    </Button>
    <InputError message={page.props.errors.passkey ?? failure} />
    {#if works === false}
      <p class="text-sm text-muted-foreground">{t("This browser doesn't make passkeys: open the link in one that does.")}</p>
    {/if}
  </div>
</div>
