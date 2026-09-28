<script lang="ts">
  import { page } from '@inertiajs/svelte'
  import AppLogoIcon from '@/components/AppLogoIcon.svelte'

  // AppLogo is the mark and the name, as the header has them.
</script>

<span class="flex items-center gap-2">
  <AppLogoIcon />
  <span class="font-semibold tracking-tight">{page.props.appName}</span>
</span>
