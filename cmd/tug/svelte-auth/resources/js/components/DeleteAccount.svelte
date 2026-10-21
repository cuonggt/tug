<script lang="ts">
  import { createForm, page, router } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
  import PasswordInput from '@/components/PasswordInput.svelte'
  import { Button } from '@/components/ui/button'
  import {
    Dialog,
    DialogClose,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogTitle,
    DialogTrigger,
  } from '@/components/ui/dialog'
  import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
  import { dismissed, getPasskey, optionsFrom } from '@/lib/passkeys'
  import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

  // DeleteAccount deletes the user's account, after asking for the password
  // once more, or for an account with none, a passkey: it can't be undone.
  let password = $state<HTMLElement | null>(null)
  let deleting = $state(false)
  let failure = $state<string>()

  async function withPasskey() {
    deleting = true
    failure = undefined
    try {
      const options = await optionsFrom(route('password.confirm.passkey.options'))
      if (!options) return
      const credential = await getPasskey(options)
      router.delete(route('profile.destroy'), { data: { credential }, preserveScroll: true, onFinish: () => (deleting = false) })
    } catch (err) {
      deleting = false
      if (!dismissed(err)) failure = t("Your browser didn't use a passkey: try again.")
    }
  }

  // The form, typed by its route's input, as tug gen writes it from the
  // Go: its errors are the fields it sends.
  const DeleteForm = createForm<Inputs['profile.destroy']>()
</script>

<section class="space-y-6">
  <Heading small title={t('Delete your account')} description={t('Your account, and everything in it, for good.')} />
  <div class="space-y-4 rounded-lg border border-destructive/30 bg-destructive/5 p-4">
    <p class="text-sm text-destructive">{t("Once it's gone, it's gone: there's no undoing it.")}</p>
    <Dialog>
      <DialogTrigger>
        {#snippet child({ props })}
          <Button {...props} variant="destructive">{t('Delete my account')}</Button>
        {/snippet}
      </DialogTrigger>
      <DialogContent>
        <DialogTitle>{t('Delete your account?')}</DialogTitle>
        {#if page.props.auth.user?.password}
          <DialogDescription>
            {t("Everything in it goes too, and you're logged out everywhere. Type your password to say you mean it.")}
          </DialogDescription>
          <DeleteForm
            action={form('profile.destroy')}
            options={{ preserveScroll: true }}
            onError={() => password?.focus()}
            resetOnError
            class="space-y-6"
          >
            {#snippet children({ errors, processing, resetAndClearErrors })}
              <div class="grid gap-2">
                <Label for="delete-password" class="sr-only">{t('Password')}</Label>
                <PasswordInput
                  id="delete-password"
                  name="password"
                  bind:ref={password}
                  autocomplete="current-password"
                  placeholder={t('Password')}
                />
                <InputError message={errors.password} />
              </div>
              <DialogFooter class="gap-2">
                <DialogClose onclick={() => resetAndClearErrors()}>
                  {#snippet child({ props })}
                    <Button {...props} type="button" variant="secondary">{t('Keep it')}</Button>
                  {/snippet}
                </DialogClose>
                <Button type="submit" variant="destructive" disabled={processing}>{t('Delete my account')}</Button>
              </DialogFooter>
            {/snippet}
          </DeleteForm>
        {:else}
          <DialogDescription>
            {t("Everything in it goes too, and you're logged out everywhere. Use your passkey to say you mean it.")}
          </DialogDescription>
          <InputError message={page.props.errors.passkey ?? failure} />
          <DialogFooter class="gap-2">
            <DialogClose>
              {#snippet child({ props })}
                <Button {...props} type="button" variant="secondary">{t('Keep it')}</Button>
              {/snippet}
            </DialogClose>
            <Button type="button" variant="destructive" disabled={deleting} onclick={withPasskey}>
              {#if deleting}
                <LoaderCircle class="animate-spin" />
              {/if}
              {t('Delete my account')}
            </Button>
          </DialogFooter>
        {/if}
      </DialogContent>
    </Dialog>
  </div>
</section>
