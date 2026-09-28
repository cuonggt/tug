<script lang="ts">
  import { Link, page } from '@inertiajs/svelte'
  import type { Snippet } from 'svelte'

  // Layout is what every page has around it: the app's name, and the flash
  // message the page after a form arrives with.
  let { children }: { children: Snippet } = $props()
</script>

<main>
  <p class="brand">
    <Link href="/">{page.props.appName}</Link>
  </p>
  {#if page.flash.success}
    <p role="status" class="flash">{page.flash.success}</p>
  {/if}
  {@render children()}
</main>
