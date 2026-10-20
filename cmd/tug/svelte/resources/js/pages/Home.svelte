<script lang="ts">
  import { createForm } from '@inertiajs/svelte'
  import Head from '../Head.svelte'
  import Layout from '../Layout.svelte'
  import { useLang } from '../lib/lang'
  import type { PageProps } from '../tug/pages'
  import { form, route, type Inputs } from '../tug/routes'

  // Home is the page main.go's home handler renders. Its props, and the
  // routes route() knows, are the Go ones: tug gen writes their TypeScript
  // into ../tug.
  let { appName, greeting }: PageProps<'Home'> = $props()

  // The form, typed by its route's input, as tug gen writes it from the
  // Go: its errors are the fields it sends.
  const HelloForm = createForm<Inputs['hello']>()

  // The page's own words, in the language of the request it came with, as
  // the server says its own: `tug lang vi` writes them into lang/vi.json,
  // to translate.
  const { t } = useLang()
</script>

<Layout>
  <Head title="Home" />
  <h1>{appName}</h1>
  <p>{greeting}</p>

  <!-- The name is checked by the server as the field is left, and again
       when the form is sent: BindValid in main.go does both. -->
  <HelloForm action={form('hello')} class="form">
    {#snippet children({ errors, processing, validate, invalid })}
      <label>
        {t("What's your name?")}
        <input name="name" aria-invalid={invalid('name')} onblur={() => validate('name')} />
      </label>
      {#if errors.name}
        <p class="error">{errors.name}</p>
      {/if}
      <button type="submit" disabled={processing}>{t('Say hello')}</button>
    {/snippet}
  </HelloForm>

  <p class="hint">
    Change <code>main.go</code> or <code>resources/js/pages/Home.svelte</code>, and the page follows.
  </p>
</Layout>
