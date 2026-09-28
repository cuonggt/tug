<script lang="ts">
  // Heading is a section's title, and a line on what it's for.
  let { title, description, small = false }: { title: string; description?: string; small?: boolean } = $props()
</script>

<header class={small ? 'space-y-1' : 'mb-8 space-y-1'}>
  <h2 class={small ? 'text-base font-medium' : 'text-2xl font-semibold tracking-tight'}>{title}</h2>
  {#if description}
    <p class="text-sm text-muted-foreground">{description}</p>
  {/if}
</header>
