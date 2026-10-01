<script setup lang="ts">
import { Form, Head } from '@inertiajs/vue3'
import Layout from '../Layout.vue'
import type { Pages, SharedProps } from '../tug/pages'
import { form, route } from '../tug/routes'

// Home is the page main.go's home handler renders. Its props, and the
// routes route() knows, are the Go ones: tug gen writes their TypeScript
// into ../tug. Vue makes a page's props from their type, which it can't
// work out from PageProps<'Home'>, so they're spelled out.
defineProps<Pages['Home'] & SharedProps>()
</script>

<template>
  <Layout>
    <Head title="Home" />
    <h1>{{ appName }}</h1>
    <p>{{ greeting }}</p>

    <!-- The name is checked by the server as the field is left, and again
         when the form is sent: BindValid in main.go does both. -->
    <Form v-slot="{ errors, processing, validate, invalid }" :action="form('hello')" class="form">
      <label>
        What's your name?
        <input name="name" :aria-invalid="invalid('name')" @blur="validate('name')" />
      </label>
      <p v-if="errors.name" class="error">{{ errors.name }}</p>
      <button type="submit" :disabled="processing">Say hello</button>
    </Form>

    <p class="hint">
      Change <code>main.go</code> or <code>resources/js/pages/Home.vue</code>, and the page follows.
    </p>
  </Layout>
</template>
