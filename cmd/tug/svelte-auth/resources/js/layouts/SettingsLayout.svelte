<script lang="ts">
  import { Link, page } from '@inertiajs/svelte'
  import type { Snippet } from 'svelte'
  import Heading from '@/components/Heading.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { Separator } from '@/components/ui/separator'
  import { cn } from '@/lib/utils'
  import { route } from '@/tug/routes'

  const pages = [
    { title: 'Profile', href: route('profile.edit') },
    { title: 'Security', href: route('security.edit') },
    { title: 'API tokens', href: route('tokens.index') },
    { title: 'Appearance', href: route('appearance.edit') },
  ]

  // SettingsLayout is around the settings pages, inside the app's layout:
  // their names, down the side, or across the top on a phone.
  let { children }: { children: Snippet } = $props()
</script>

<Heading title="Settings" description="Your profile, your account's security, your API tokens, and how the app looks" />
<div class="flex flex-col gap-6 lg:flex-row lg:gap-12">
  <aside class="lg:w-48">
    <nav aria-label="Settings" class="flex gap-1 lg:flex-col">
      {#each pages as settings (settings.href)}
        <Link
          href={settings.href}
          class={cn(
            buttonVariants({ variant: 'ghost', size: 'sm' }),
            'justify-start',
            page.url.startsWith(settings.href) && 'bg-muted',
          )}
        >
          {settings.title}
        </Link>
      {/each}
    </nav>
  </aside>
  <!-- A line to see, not a separator to hear: bits-ui's announces itself
       unless it's decorative. -->
  <Separator class="lg:hidden" decorative />
  <div class="max-w-xl flex-1 space-y-12">{@render children()}</div>
</div>
