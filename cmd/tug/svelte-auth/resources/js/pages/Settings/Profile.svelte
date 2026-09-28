<script lang="ts">
  import { Form, Link } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import DeleteAccount from '@/components/DeleteAccount.svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import type { PageProps } from '@/tug/pages'
  import { route } from '@/tug/routes'

  // Profile changes the user's name and email. A new email is mailed a link,
  // and isn't verified until it's followed: updateProfile in settings.go.
  let { user }: PageProps<'Settings/Profile'> = $props()
</script>

<Head title="Profile" />
<section class="space-y-6">
  <Heading small title="Profile" description="Your name, and the email we reach you at." />
  <Form action={route('profile.update')} method="patch" options={{ preserveScroll: true }} class="space-y-6">
    {#snippet children({ errors, processing })}
      <div class="grid gap-2">
        <Label for="name">Name</Label>
        <Input id="name" name="name" value={user.name} autocomplete="name" required aria-invalid={!!errors.name} />
        <InputError message={errors.name} />
      </div>
      <div class="grid gap-2">
        <Label for="email">Email</Label>
        <Input
          id="email"
          name="email"
          type="email"
          value={user.email}
          autocomplete="username"
          required
          aria-invalid={!!errors.email}
        />
        <InputError message={errors.email} />
        {#if !user.emailVerifiedAt}
          <p class="text-sm text-muted-foreground">
            This email isn't verified yet.
            <Link
              href={route('verification.send')}
              method="post"
              as="button"
              preserveScroll
              class="cursor-pointer text-foreground underline underline-offset-4"
            >
              Send the link again
            </Link>
          </p>
        {/if}
      </div>
      <Button type="submit" disabled={processing}>Save</Button>
    {/snippet}
  </Form>
</section>
<DeleteAccount />
