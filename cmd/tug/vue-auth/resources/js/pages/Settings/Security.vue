<script setup lang="ts">
import { Form, Head } from '@inertiajs/vue3'
import Heading from '@/components/Heading.vue'
import InputError from '@/components/InputError.vue'
import PasskeySettings from '@/components/PasskeySettings.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TwoFactorSettings from '@/components/TwoFactorSettings.vue'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import type { Pages, SharedProps } from '@/tug/pages'
import { form } from '@/tug/routes'

// Security changes the user's password, which logs them out everywhere
// else, turns two-factor logins on and off, and keeps their passkeys.
const props = defineProps<Pages['Settings/Security'] & SharedProps>()
</script>

<template>
  <Head title="Security" />
  <section class="space-y-6">
    <Heading small title="Password" description="A new one logs you out everywhere else, such as a lost laptop." />
    <Form
      v-slot="{ errors, processing }"
      :action="form('user-password.update')"
      :options="{ preserveScroll: true }"
      reset-on-error
      reset-on-success
      class="space-y-6"
    >
      <div class="grid gap-2">
        <Label for="current_password">Current password</Label>
        <PasswordInput id="current_password" name="current_password" autocomplete="current-password" required />
        <InputError :message="errors.current_password" />
      </div>
      <div class="grid gap-2">
        <Label for="password">New password</Label>
        <PasswordInput id="password" name="password" autocomplete="new-password" required />
        <InputError :message="errors.password" />
      </div>
      <div class="grid gap-2">
        <Label for="password_confirmation">New password again</Label>
        <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
        <InputError :message="errors.password_confirmation" />
      </div>
      <Button type="submit" :disabled="processing">Change the password</Button>
    </Form>
  </section>
  <PasskeySettings v-bind="props" />
  <TwoFactorSettings v-bind="props" />
</template>
