<script setup lang="ts">
import { Form, Head } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import InputError from '@/components/InputError.vue'
import PasswordInput from '@/components/PasswordInput.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import type { Pages, SharedProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// ResetPassword is where the link in a reset mail leads: its token, and
// the email it went to, come in the props.
defineOptions({
  layout: { title: 'Choose a new password', description: 'It logs you out everywhere you were logged in.' },
})
defineProps<Pages['Auth/ResetPassword'] & SharedProps>()
</script>

<template>
  <Head title="Choose a new password" />
  <Form
    v-slot="{ errors, processing }"
    :action="route('password.store')"
    method="post"
    :reset-on-error="['password', 'password_confirmation']"
    class="flex flex-col gap-6"
  >
    <input type="hidden" name="token" :value="token" />
    <div class="grid gap-2">
      <Label for="email">Email</Label>
      <Input
        id="email"
        name="email"
        type="email"
        autocomplete="username"
        :model-value="email"
        readonly
        :aria-invalid="!!errors.email"
      />
      <InputError :message="errors.email" />
    </div>
    <div class="grid gap-2">
      <Label for="password">New password</Label>
      <PasswordInput id="password" v-focus name="password" autocomplete="new-password" required />
      <InputError :message="errors.password" />
    </div>
    <div class="grid gap-2">
      <Label for="password_confirmation">New password again</Label>
      <PasswordInput id="password_confirmation" name="password_confirmation" autocomplete="new-password" required />
      <InputError :message="errors.password_confirmation" />
    </div>
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Set the password</Button>
  </Form>
  <p class="mt-6 text-center text-sm text-muted-foreground">Has the link stopped working? <TextLink :href="route('password.request')">Ask for another</TextLink></p>
</template>
