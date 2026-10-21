<script setup lang="ts">
import { createForm, Head } from '@inertiajs/vue3'
import { LoaderCircle } from '@lucide/vue'
import InputError from '@/components/InputError.vue'
import TextLink from '@/components/TextLink.vue'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
  import { useLang } from '@/lib/lang'
import { vFocus } from '@/lib/focus'
  import { text } from '@/tug/lang'
import { form, route, type Inputs } from '@/tug/routes'

  // The page says its own words in the request's language.
  const { t } = useLang()

// ForgotPassword mails a link that sets a new password. In development,
// without MAIL_HOST in .env, the mail is kept in the app's mailbox, at
// /_tug/mail, which tug dev's terminal links to.
defineOptions({
  layout: {
    title: text('Forgotten your password, or lost your passkey?'),
    description: text("Say the email of your account, and we'll mail you a link to choose a new password."),
  },
})

// The form, typed by its route's input, as tug gen writes it from the Go:
// its errors are the fields it sends.
const ResetLinkForm = createForm<Inputs['password.email']>()
</script>

<template>
  <Head :title="t('Forgotten password')" />
  <ResetLinkForm
    v-slot="{ errors, processing }"
    :action="form('password.email')"
    reset-on-success
    class="flex flex-col gap-6"
  >
    <div class="grid gap-2">
      <Label for="email">{{ t('Email') }}</Label>
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
    <Button type="submit" class="w-full" :disabled="processing"><LoaderCircle v-if="processing" class="animate-spin" />{{ t('Mail me a link') }}</Button>
  </ResetLinkForm>
  <p class="mt-6 text-center text-sm text-muted-foreground">{{ t('Remembered it?') }} <TextLink :href="route('login')">{{ t('Log in') }}</TextLink></p>
</template>
