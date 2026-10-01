<script lang="ts">
  import { Form, page, router } from '@inertiajs/svelte'
  import Check from '@lucide/svelte/icons/check'
  import Copy from '@lucide/svelte/icons/copy'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import ShieldCheck from '@lucide/svelte/icons/shield-check'
  import QR from '@svelte-put/qr/svg/QR.svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
  import { Badge } from '@/components/ui/badge'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import type { PageProps } from '@/tug/pages'
  import { form, route } from '@/tug/routes'

  // TwoFactorSettings turns two-factor logins on and off: off, a button to
  // start; starting, a QR code for the user's authenticator app and a code
  // back from it; on, their recovery codes, and a button to stop. The
  // handlers are in twofactor.go.
  let { user, setup, recoveryCodes }: PageProps<'Settings/Security'> = $props()
  // The codes come once in the flash as two-factor logins are turned on, or
  // new ones are made, and the next visit empties it, as the bell's reload
  // does when a notification rings it: kept here, they're shown until the
  // page is left. A partial reload that asks for them brings them again.
  let kept = $state<string[]>()
  $effect(() => {
    if (page.flash.recoveryCodes) kept = page.flash.recoveryCodes
  })
  let codes = $derived(page.flash.recoveryCodes ?? kept ?? recoveryCodes)

  let copied = $state(false)
  async function copy(codes: string[]) {
    await navigator.clipboard.writeText(codes.join('\n'))
    copied = true
    setTimeout(() => (copied = false), 2000)
  }
</script>

<section class="space-y-6">
  <div class="flex items-start justify-between gap-4">
    <Heading
      small
      title="Two-factor logins"
      description="A code from an app on your phone, as well as your password, each time you log in."
    />
    <Badge variant={user.twoFactor ? 'default' : 'secondary'}>{user.twoFactor ? 'On' : 'Off'}</Badge>
  </div>

  {#if user.twoFactor}
    {#if codes}
      {@render recoveryCodesList(codes)}
    {:else}
      <div class="space-y-3">
        <p class="text-sm text-muted-foreground">
          Recovery codes log you in when your phone is lost: one code, one login.
        </p>
        <Button variant="outline" onclick={() => router.reload({ only: ['recoveryCodes'] })}>
          Show my recovery codes
        </Button>
      </div>
    {/if}
    <Form action={form('two-factor.disable')} options={{ preserveScroll: true }}>
      {#snippet children({ processing })}
        <Button type="submit" variant="destructive" disabled={processing}>Turn two-factor logins off</Button>
      {/snippet}
    </Form>
  {:else if setup}
    <div class="space-y-6">
      <ol class="list-decimal space-y-2 pl-5 text-sm text-muted-foreground">
        <li>Scan this with an authenticator app, such as 1Password, Google Authenticator or Authy.</li>
        <li>Type the code it shows, to be sure it's set up.</li>
      </ol>
      <div class="flex flex-col items-start gap-4 sm:flex-row sm:items-center">
        <div class="rounded-lg border bg-white p-3">
          <!-- The QR is drawn in the text's colour: black, on the white card,
               in either appearance. With no logo over it, it needs the
               least error correction, which makes the fewest modules. -->
          <QR
            data={setup.url}
            width={160}
            height={160}
            margin={0}
            correction="L"
            class="text-black"
            role="img"
            aria-label="The QR code for your authenticator app"
          />
        </div>
        <div class="space-y-1 text-sm">
          <p class="text-muted-foreground">Can't scan it? Type this key into the app instead:</p>
          <p class="font-mono text-base tracking-wider break-all select-all">
            {setup.secret.match(/.{1,4}/g)?.join(' ')}
          </p>
        </div>
      </div>
      <Form
        action={form('two-factor.confirm')}
        resetOnError
        options={{ preserveScroll: true }}
        class="space-y-4"
      >
        {#snippet children({ errors, processing })}
          <div class="grid max-w-48 gap-2">
            <Label for="code">The code from the app</Label>
            <Input
              id="code"
              name="code"
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength={7}
              placeholder="123456"
              required
              autofocus
              class="font-mono tracking-widest"
              aria-invalid={!!errors.code}
            />
          </div>
          <InputError message={errors.code} />
          <div class="flex gap-2">
            <Button type="submit" disabled={processing}>
              {#if processing}
                <LoaderCircle class="animate-spin" />
              {/if}
              Turn on
            </Button>
            <Button
              type="button"
              variant="ghost"
              onclick={() => router.delete(route('two-factor.disable'), { preserveScroll: true })}
            >
              Cancel
            </Button>
          </div>
        {/snippet}
      </Form>
    </div>
  {:else}
    <Form action={form('two-factor.enable')} options={{ preserveScroll: true }}>
      {#snippet children({ processing })}
        <Button type="submit" disabled={processing}>
          <ShieldCheck />
          Turn two-factor logins on
        </Button>
      {/snippet}
    </Form>
  {/if}
</section>

<!-- recoveryCodesList lists the user's recovery codes, to copy somewhere
     safe, and makes new ones. -->
{#snippet recoveryCodesList(codes: string[])}
  <div class="space-y-3">
    <p class="text-sm text-muted-foreground">
      Keep these somewhere safe, such as a password manager. Each logs you in once, when your phone is lost.
    </p>
    {#if codes.length > 0}
      <ul class="grid grid-cols-2 gap-x-6 gap-y-1 rounded-lg bg-muted p-4 font-mono text-sm">
        {#each codes as code (code)}
          <li>{code}</li>
        {/each}
      </ul>
    {:else}
      <p class="rounded-lg bg-muted p-4 text-sm">You've used them all: make new ones.</p>
    {/if}
    <div class="flex flex-wrap gap-2">
      {#if codes.length > 0}
        <Button type="button" variant="outline" onclick={() => copy(codes)}>
          {#if copied}
            <Check />
          {:else}
            <Copy />
          {/if}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      {/if}
      <Form action={form('two-factor.recovery-codes')} options={{ preserveScroll: true }}>
        {#snippet children({ processing })}
          <Button type="submit" variant="outline" disabled={processing}>Make new codes</Button>
        {/snippet}
      </Form>
    </div>
  </div>
{/snippet}
