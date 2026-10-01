<script setup lang="ts">
import { Form, Head } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import InputError from '@/components/InputError.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import { form, route } from '@/tug/routes'

// ForgotPassword mails a link that sets a new password. In development,
// without MAIL_HOST in .env, the mail is written to tug dev's terminal.
defineOptions({
  layout: {
    title: 'Forgotten your password?',
    description: "Say the email of your account, and we'll mail you a link to choose a new one.",
  },
})
</script>

<template>
  <Head title="Forgotten password" />
  <Form
    v-slot="{ errors, processing }"
    :action="form('password.email')"
    reset-on-success
    class="flex flex-col gap-6"
  >
    <div class="grid gap-2">
      <Label for="email">Email</Label>
      <Input
        id="email"
        v-focus
        name="email"
        type="email"
        autocomplete="email"
        placeholder="you@example.com"
        required
        :aria-invalid="!!errors.email"
      />
      <InputError :message="errors.email" />
    </div>
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />Mail me a link</Button>
  </Form>
  <p class="mt-6 text-center text-sm text-muted-foreground">Remembered it? <TextLink :href="route('login')">Log in</TextLink></p>
</template>
