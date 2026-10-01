<script lang="ts">
  import { Link, page } from '@inertiajs/svelte'
  import { route } from '@/tug/routes'

  // ActingBanner is the line at the top of each page while an admin acts as
  // its user, to see the app as they do (actAs, in admin.go): whose the
  // pages are, and the way back to the admin's own account.
  let auth = $derived(page.props.auth)
</script>

{#if auth.acting && auth.user}
  <div class="bg-primary text-primary-foreground">
    <div class="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-x-4 gap-y-1 px-4 py-2 text-sm">
      <p>
        You're acting as <span class="font-medium">{auth.user.name}</span>, {auth.user.email}.
      </p>
      <Link href={route('acting.stop')} method="post" as="button" class="cursor-pointer font-medium underline underline-offset-4">
        Back to your account
      </Link>
    </div>
  </div>
{/if}
