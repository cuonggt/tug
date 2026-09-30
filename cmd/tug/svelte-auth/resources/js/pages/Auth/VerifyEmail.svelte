<script module lang="ts">
  export const layout = { title: 'Verify your email', description: 'One more step, and the app is yours to use.' }
</script>

<script lang="ts">
  import { Form, Link, page, router } from '@inertiajs/svelte'
  import LoaderCircle from '@lucide/svelte/icons/loader-circle'
  import { onMount } from 'svelte'
  import Head from '@/Head.svelte'
  import TextLink from '@/components/TextLink.svelte'
  import { Button } from '@/components/ui/button'
  import { listen } from '@/lib/broadcasts'
  import { route } from '@/tug/routes'

  // VerifyEmail is where the pages for verified users send someone who
  // hasn't followed the link mailed to them yet. In development, without
  // MAIL_HOST in .env, the mail is written to tug dev's terminal.

  // Verified in another tab, or on the phone the mail went to, the email
  // moves this page on: the app says so on the user's own channel, and the
  // page reloads, which its handler, in verify.go, answers with the
  // dashboard once the email is verified.
  onMount(() => listen('verified', () => router.reload()))
</script>

<Head title="Verify your email" />
<div class="flex flex-col gap-4 text-center">
  <p class="text-sm text-muted-foreground">
    We mailed a link to <span class="font-medium text-foreground">{page.props.auth.user?.email}</span>. Follow it to
    verify the email is yours; it works for a day.
  </p>
  <Form action={route('verification.send')} method="post">
    {#snippet children({ processing })}
      <Button type="submit" variant="secondary" class="w-full" disabled={processing}>
        {#if processing}
          <LoaderCircle class="animate-spin" />
        {/if}
        Send another link
      </Button>
    {/snippet}
  </Form>
  <p class="text-sm text-muted-foreground">
    The wrong email? <TextLink href={route('profile.edit')}>Change it</TextLink>, or
    <Link href={route('logout')} method="post" as="button" class="cursor-pointer underline underline-offset-4">log out</Link>.
  </p>
</div>
