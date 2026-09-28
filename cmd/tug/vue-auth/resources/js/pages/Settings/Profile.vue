<script setup lang="ts">
import { Form, Head, Link } from '@inertiajs/vue3'
import DeleteAccount from '@/components/DeleteAccount.vue'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Profile changes the user's name and email. A new email is mailed a link,
// and isn't verified until it's followed: updateProfile in settings.go.
defineProps<Pages['Settings/Profile'] & SharedProps>()
</script>

<template>
  <Head title="Profile" />
  <section class="space-y-6">
    <Heading small title="Profile" description="Your name, and the email we reach you at." />
    <Form
      v-slot="{ errors, processing }"
      :action="route('profile.update')"
      method="patch"
      :options="{ preserveScroll: true }"
      class="space-y-6"
    >
      <div class="grid gap-2">
        <Label for="name">Name</Label>
        <Input
          id="name"
          name="name"
          :default-value="user.name"
          autocomplete="name"
          required
          :aria-invalid="!!errors.name"
        />
        <InputError :message="errors.name" />
      </div>
      <div class="grid gap-2">
        <Label for="email">Email</Label>
        <Input
          id="email"
          name="email"
          type="email"
          :default-value="user.email"
          autocomplete="username"
          required
          :aria-invalid="!!errors.email"
        />
        <InputError :message="errors.email" />
        <p v-if="!user.emailVerifiedAt" class="text-sm text-muted-foreground">This email isn't verified yet.
          <Link
            :href="route('verification.send')"
            method="post"
            as="button"
            preserve-scroll
            class="cursor-pointer text-foreground underline underline-offset-4"
          >Send the link again</Link></p>
      </div>
      <Button type="submit" :disabled="processing">Save</Button>
    </Form>
  </section>
  <DeleteAccount />
</template>
