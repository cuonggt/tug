<script setup lang="ts">
import { createForm, Head } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import InputError from '@/components/InputError.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { vFocus } from '@/lib/focus'
import { form, route, type Inputs } from '@/tug/routes'

// ForgotPassword mails a link that sets a new password. In development,
// without MAIL_HOST in .env, the mail is written to tug dev's terminal.
defineOptions({
  layout: {
    title: 'Forgotten your password, or lost your passkey?',
    description: "Say the email of your account, and we'll mail you a link to choose a new password.",
  },
})

// The form, typed by its route's input, as tug gen writes it from the Go:
// its errors are the fields it sends.
const ResetLinkForm = createForm<Inputs['password.email']>()
</script>

<template>
  <Head title="Forgotten password" />
  <ResetLinkForm
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
  </ResetLinkForm>
  <p class="mt-6 text-center text-sm text-muted-foreground">Remembered it? <TextLink :href="route('login')">Log in</TextLink></p>
</template>
