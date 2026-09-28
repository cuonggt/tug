<script lang="ts">
  import { page } from '@inertiajs/svelte'

  // Head gives a page its title, with the app's name after it, as
  // Inertia's Head does in React and Vue.
  let { title = '' }: { title?: string } = $props()
</script>

<svelte:head>
  <title>{title ? `${title} · ${page.props.appName}` : page.props.appName}</title>
</svelte:head>
