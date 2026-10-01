<script module lang="ts">
  export const layout = {
    title: 'Two-factor login',
    description: 'The six-digit code your authenticator app shows for this account.',
  }
</script>

<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import type { Attachment } from 'svelte/attachments'
  import Head from '@/Head.svelte'
  import InputError from '@/components/InputError.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { form } from '@/tug/routes'

  // TwoFactorChallenge is the second step of logging in, for a user who has
  // turned two-factor logins on: a code from their authenticator app, or,
  // with the phone lost, one of their recovery codes.
  let recovery = $state(false)

  // focus has the field the button below swaps in take the focus from the
  // button: Svelte's autofocus moves the focus only when nothing has it.
  const focus: Attachment<HTMLElement> = (field) => field.focus()
</script>

<Head title="Two-factor login" />
<!-- The form is made again as they switch, with no errors from the other,
     and the field it shows focused. -->
{#key recovery}
  <Form action={form('two-factor.login.store')} resetOnError class="flex flex-col gap-6">
    {#snippet children({ errors, processing })}
      {#if recovery}
        <div class="grid gap-2">
          <Label for="recovery_code">Recovery code</Label>
          <Input
            id="recovery_code"
            name="recovery_code"
            autocomplete="off"
            placeholder="abcde-fghij"
            required
            autofocus
            {@attach focus}
            aria-invalid={!!errors.recovery_code}
          />
          <InputError message={errors.recovery_code} />
        </div>
      {:else}
        <div class="grid gap-2">
          <Label for="code">Code</Label>
          <Input
            id="code"
            name="code"
            inputmode="numeric"
            autocomplete="one-time-code"
            pattern="[0-9 ]*"
            maxlength={7}
            placeholder="123456"
            required
            autofocus
            {@attach focus}
            class="text-center font-mono text-lg tracking-[0.3em]"
            aria-invalid={!!errors.code}
          />
          <InputError message={errors.code} />
        </div>
      {/if}
      <Button type="submit" class="w-full" disabled={processing}>
        {#if processing}
          <LoaderCircle class="animate-spin" />
        {/if}
        Log in
      </Button>
    {/snippet}
  </Form>
{/key}
<p class="mt-6 text-center text-sm text-muted-foreground">
  {recovery ? 'Found your phone?' : 'Lost your phone?'}
  <button
    type="button"
    onclick={() => (recovery = !recovery)}
    class="cursor-pointer text-foreground underline underline-offset-4"
  >
    {recovery ? 'Use a code from the app' : 'Use a recovery code'}
  </button>
</p>
