<script lang="ts">
  import { Form } from '@inertiajs/svelte'
  import Head from '../Head.svelte'
  import Layout from '../Layout.svelte'
  import type { PageProps } from '../tug/pages'
  import { route } from '../tug/routes'

  // Home is the page main.go's home handler renders. Its props, and the
  // routes route() knows, are the Go ones: tug gen writes their TypeScript
  // into ../tug.
  let { appName, greeting }: PageProps<'Home'> = $props()
</script>

<Layout>
  <Head title="Home" />
  <h1>{appName}</h1>
  <p>{greeting}</p>

  <!-- The name is checked by the server as the field is left, and again
       when the form is sent: BindValid in main.go does both. -->
  <Form action={route('hello')} method="post" class="form">
    {#snippet children({ errors, processing, validate, invalid })}
      <label>
        What's your name?
        <input name="name" aria-invalid={invalid('name')} onblur={() => validate('name')} />
      </label>
      {#if errors.name}
        <p class="error">{errors.name}</p>
      {/if}
      <button type="submit" disabled={processing}>Say hello</button>
    {/snippet}
  </Form>

  <p class="hint">
    Change <code>main.go</code> or <code>resources/js/pages/Home.svelte</code>, and the page follows.
  </p>
</Layout>
