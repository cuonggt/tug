<script lang="ts">
  import { Form } from '@inertiajs/svelte'
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
  import { form } from '@/tug/routes'

  // DeleteAccount deletes the user's account, after asking for the password
  // once more: it can't be undone.
  let password = $state<HTMLElement | null>(null)
</script>

<section class="space-y-6">
  <Heading small title="Delete your account" description="Your account, and everything in it, for good." />
  <div class="space-y-4 rounded-lg border border-destructive/30 bg-destructive/5 p-4">
    <p class="text-sm text-destructive">Once it's gone, it's gone: there's no undoing it.</p>
    <Dialog>
      <DialogTrigger>
        {#snippet child({ props })}
          <Button {...props} variant="destructive">Delete my account</Button>
        {/snippet}
      </DialogTrigger>
      <DialogContent>
        <DialogTitle>Delete your account?</DialogTitle>
        <DialogDescription>
          Everything in it goes too, and you're logged out everywhere. Type your password to say you mean it.
        </DialogDescription>
        <Form
          action={form('profile.destroy')}
          options={{ preserveScroll: true }}
          onError={() => password?.focus()}
          resetOnError
          class="space-y-6"
        >
          {#snippet children({ errors, processing, resetAndClearErrors })}
            <div class="grid gap-2">
              <Label for="delete-password" class="sr-only">Password</Label>
              <PasswordInput
                id="delete-password"
                name="password"
                bind:ref={password}
                autocomplete="current-password"
                placeholder="Password"
              />
              <InputError message={errors.password} />
            </div>
            <DialogFooter class="gap-2">
              <DialogClose onclick={() => resetAndClearErrors()}>
                {#snippet child({ props })}
                  <Button {...props} type="button" variant="secondary">Keep it</Button>
                {/snippet}
              </DialogClose>
              <Button type="submit" variant="destructive" disabled={processing}>Delete my account</Button>
            </DialogFooter>
          {/snippet}
        </Form>
      </DialogContent>
    </Dialog>
  </div>
</section>
