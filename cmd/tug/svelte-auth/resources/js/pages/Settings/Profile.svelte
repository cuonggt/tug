<script lang="ts">
  import { createForm, Link, router } from '@inertiajs/svelte'
  import Head from '@/Head.svelte'
  import DeleteAccount from '@/components/DeleteAccount.svelte'
  import Heading from '@/components/Heading.svelte'
  import InputError from '@/components/InputError.svelte'
  import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
  import { Button } from '@/components/ui/button'
  import { Input } from '@/components/ui/input'
  import { Label } from '@/components/ui/label'
  import { initials } from '@/lib/utils'
  import type { PageProps } from '@/tug/pages'
  import { form, route, type Inputs } from '@/tug/routes'

  // Profile changes the user's name and email, and their photo. A new email
  // is mailed a link, and isn't verified until it's followed: updateProfile
  // in settings.go. The photo goes up as the form's file, with its progress
  // shown, and replaces the one before: updatePhoto in photos.go.
  let { user }: PageProps<'Settings/Profile'> = $props()

  // The forms, typed by their routes' inputs, as tug gen writes them from
  // the Go: their errors are the fields they send.
  const ProfileForm = createForm<Inputs['profile.update']>()
  const PhotoForm = createForm<Inputs['profile.photo.update']>()
</script>

<Head title="Profile" />
<section class="space-y-6">
  <Heading small title="Profile" description="Your name, and the email we reach you at." />
  <ProfileForm action={form('profile.update')} options={{ preserveScroll: true }} class="space-y-6">
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
  </ProfileForm>
</section>
<section class="space-y-6">
  <Heading small title="Photo" description="Shown in place of your initials: a PNG, JPEG or WebP, of 2 MB at most." />
  <div class="flex items-start gap-6">
    {#key user.photo}
      <Avatar class="size-16">
        {#if user.photo}
          <AvatarImage src={user.photo} alt="Your photo" />
        {/if}
        <AvatarFallback class="text-lg font-medium">{initials(user.name)}</AvatarFallback>
      </Avatar>
    {/key}
    <PhotoForm
      action={form('profile.photo.update')}
      options={{ preserveScroll: true }}
      resetOnSuccess
      class="grid flex-1 gap-2"
    >
      {#snippet children({ errors, processing, progress })}
        <Label for="photo">Choose a photo</Label>
        <Input
          id="photo"
          name="photo"
          type="file"
          accept="image/png,image/jpeg,image/webp"
          required
          aria-invalid={!!errors.photo}
        />
        <InputError message={errors.photo} />
        {#if progress}
          <progress value={progress.percentage} max="100" aria-label="Uploading" class="w-full accent-primary"></progress>
        {/if}
        <div class="flex gap-2">
          <Button type="submit" disabled={processing}>Upload</Button>
          {#if user.photo}
            <Button
              type="button"
              variant="outline"
              onclick={() => router.delete(route('profile.photo.destroy'), { preserveScroll: true })}
            >
              Remove
            </Button>
          {/if}
        </div>
      {/snippet}
    </PhotoForm>
  </div>
</section>
<DeleteAccount />
