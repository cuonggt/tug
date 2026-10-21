<script lang="ts">
  import { Link, page } from '@inertiajs/svelte'
  import type { Snippet } from 'svelte'
  import Heading from '@/components/Heading.svelte'
  import { buttonVariants } from '@/components/ui/button'
  import { Separator } from '@/components/ui/separator'
  import { useLang } from '@/lib/lang'
  import { cn } from '@/lib/utils'
  import { text } from '@/tug/lang'
  import { route } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

  const pages = [
    { title: text('Profile'), href: route('profile.edit') },
    { title: text('Security'), href: route('security.edit') },
    { title: text('API tokens'), href: route('tokens.index') },
    { title: text('Appearance'), href: route('appearance.edit') },
  ]

  // SettingsLayout is around the settings pages, inside the app's layout:
  // their names, down the side, or across the top on a phone.
  let { children }: { children: Snippet } = $props()
</script>

<Heading title={t('Settings')} description={t("Your profile, your account's security, your API tokens, and how the app looks")} />
<div class="flex flex-col gap-6 lg:flex-row lg:gap-12">
  <aside class="lg:w-48">
    <nav aria-label={t('Settings')} class="flex gap-1 lg:flex-col">
      {#each pages as settings (settings.href)}
        <Link
          href={settings.href}
          class={cn(
            buttonVariants({ variant: 'ghost', size: 'sm' }),
            'justify-start',
            page.url.startsWith(settings.href) && 'bg-muted',
          )}
        >
          {t(settings.title)}
        </Link>
      {/each}
    </nav>
  </aside>
  <!-- A line to see, not a separator to hear: bits-ui's announces itself
       unless it's decorative. -->
  <Separator class="lg:hidden" decorative />
  <div class="max-w-xl flex-1 space-y-12">{@render children()}</div>
</div>
