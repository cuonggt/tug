<script lang="ts">
  import Head from '@/Head.svelte'
  import AppearanceTabs from '@/components/AppearanceTabs.svelte'
  import Heading from '@/components/Heading.svelte'

  // Appearance picks light, dark, or the system's, for this browser.
</script>

<Head title="Appearance" />
<section class="space-y-6">
  <Heading small title="Appearance" description="How the app looks in this browser." />
  <AppearanceTabs />
</section>
