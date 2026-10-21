<script lang="ts">
  import Head from '@/Head.svelte'
  import AppearanceTabs from '@/components/AppearanceTabs.svelte'
  import Heading from '@/components/Heading.svelte'
  import { useLang } from '@/lib/lang'

  // The page says its own words in the request's language.
  const { t } = useLang()

  // Appearance picks light, dark, or the system's, for this browser.
</script>

<Head title={t('Appearance')} />
<section class="space-y-6">
  <Heading small title={t('Appearance')} description={t('How the app looks in this browser.')} />
  <AppearanceTabs />
</section>
