<script lang="ts">
  import { Link } from '@inertiajs/svelte'
  import Head from '../Head.svelte'
  import Layout from '../Layout.svelte'
  import type { PageProps } from '../tug/pages'
  import { route } from '../tug/routes'

  const titles: Record<number, string> = {
    403: 'Not allowed',
    404: 'Not found',
    500: 'Something went wrong',
  }

  // Error is the page tug shows errors with (Config.ErrorPage in main.go),
  // with the response's own status.
  let { status, message }: PageProps<'Error'> = $props()
  let title = $derived(titles[status] ?? 'Something went wrong')
</script>

<Layout>
  <Head {title} />
  <h1>{title}</h1>
  <p>{status}: {message}</p>
  <p>
    <Link href={route('home')}>Back home</Link>
  </p>
</Layout>
